-- 价格设置归属价格配置，存量分组值写入各自的配置。升级前停止旧实例。
ALTER TABLE pricing_configs
 ADD COLUMN IF NOT EXISTS peak_rate_enabled boolean DEFAULT false NOT NULL,
 ADD COLUMN IF NOT EXISTS peak_start varchar(5) DEFAULT '' NOT NULL,
 ADD COLUMN IF NOT EXISTS peak_end varchar(5) DEFAULT '' NOT NULL,
 ADD COLUMN IF NOT EXISTS peak_rate_multiplier numeric(10,4) DEFAULT 1 NOT NULL,
 ADD COLUMN IF NOT EXISTS long_context_pricing_enabled boolean DEFAULT true NOT NULL,
 ADD COLUMN IF NOT EXISTS free_openai_fast boolean DEFAULT false NOT NULL,
 ADD COLUMN IF NOT EXISTS batch_image_discount_multiplier numeric(10,4) DEFAULT 0.5 NOT NULL,
 ADD COLUMN IF NOT EXISTS batch_image_hold_multiplier numeric(10,4) DEFAULT 0.6 NOT NULL,
 ADD COLUMN IF NOT EXISTS web_search_price_per_call numeric(20,8),
 ADD COLUMN IF NOT EXISTS search_price_per_1k numeric(20,8),
 ADD COLUMN IF NOT EXISTS audio_realtime_price_per_min numeric(20,8),
 ADD COLUMN IF NOT EXISTS audio_tts_price_per_million_chars numeric(20,8),
 ADD COLUMN IF NOT EXISTS audio_stt_price_per_hour numeric(20,8);


-- 每个分组获得独立价格配置，承接分组价卡、图片单价和计费设置。
DO $$
DECLARE
 g RECORD;
 base_config BIGINT;
 config_id BIGINT;
 card JSONB;
 cards JSONB;
 iv JSONB;
 row_id BIGINT;
 legacy JSONB;
 settings JSONB;
 image_models JSONB;
BEGIN
 IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='groups' AND column_name='model_pricing') THEN
  RETURN;
 END IF;
 FOR g IN SELECT current_group.*, archive.original_group
  FROM groups current_group JOIN platform_independent_group_archive archive ON archive.group_id=current_group.id
  WHERE current_group.deleted_at IS NULL ORDER BY current_group.id
 LOOP
  legacy := g.original_group;
  SELECT pricing_config_id INTO base_config FROM pricing_config_groups WHERE group_id=g.id ORDER BY pricing_config_id LIMIT 1;
  INSERT INTO pricing_configs(name,description,status,billing_model_source)
  VALUES(g.name || '（价格配置）','分组价格与计费设置','active',COALESCE((SELECT billing_model_source FROM pricing_configs WHERE id=base_config),'group_mapped'))
  RETURNING id INTO config_id;
  settings := jsonb_build_object(
   'peak_rate_enabled',COALESCE(legacy->'peak_rate_enabled','false'),
   'peak_start',COALESCE(legacy->'peak_start','""'),
   'peak_end',COALESCE(legacy->'peak_end','""'),
   'peak_rate_multiplier',COALESCE(legacy->'peak_rate_multiplier','1'),
   'long_context_pricing_enabled',COALESCE(legacy->'long_context_pricing_enabled','true'),
   'free_openai_fast',COALESCE(legacy->'free_openai_fast','false'),
   'batch_image_discount_multiplier',COALESCE(legacy->'batch_image_discount_multiplier','0.5'),
   'batch_image_hold_multiplier',COALESCE(legacy->'batch_image_hold_multiplier','0.6'),
   'web_search_price_per_call',legacy->'web_search_price_per_call',
   'search_price_per_1k',legacy->'search_price_per_1k',
   'audio_realtime_price_per_min',legacy->'audio_realtime_price_per_min',
   'audio_tts_price_per_million_chars',legacy->'audio_tts_price_per_million_chars',
   'audio_stt_price_per_hour',legacy->'audio_stt_price_per_hour');
  UPDATE pricing_configs SET
   peak_rate_enabled=COALESCE((settings->>'peak_rate_enabled')::boolean,false),
   peak_start=COALESCE(settings->>'peak_start',''),peak_end=COALESCE(settings->>'peak_end',''),
   peak_rate_multiplier=COALESCE((settings->>'peak_rate_multiplier')::numeric,1),
   long_context_pricing_enabled=COALESCE((settings->>'long_context_pricing_enabled')::boolean,true),
   free_openai_fast=COALESCE((settings->>'free_openai_fast')::boolean,false),
   batch_image_discount_multiplier=COALESCE((settings->>'batch_image_discount_multiplier')::numeric,0.5),
   batch_image_hold_multiplier=COALESCE((settings->>'batch_image_hold_multiplier')::numeric,0.6),
   web_search_price_per_call=(settings->>'web_search_price_per_call')::numeric,
   search_price_per_1k=(settings->>'search_price_per_1k')::numeric,
   audio_realtime_price_per_min=(settings->>'audio_realtime_price_per_min')::numeric,
   audio_tts_price_per_million_chars=(settings->>'audio_tts_price_per_million_chars')::numeric,
   audio_stt_price_per_hour=(settings->>'audio_stt_price_per_hour')::numeric
  WHERE id=config_id;
  cards:=COALESCE(g.model_pricing,'[]');
  -- 型号价卡排在共享价卡之前，图片兜底价使用型号匹配后的剩余范围。
  IF base_config IS NOT NULL THEN
   cards:=cards || COALESCE((SELECT jsonb_agg(to_jsonb(p) || jsonb_build_object('intervals',COALESCE((SELECT jsonb_agg(to_jsonb(i) ORDER BY i.sort_order,i.id) FROM pricing_config_pricing_intervals i WHERE i.pricing_id=p.id),'[]')) ORDER BY p.id) FROM pricing_config_model_pricing p WHERE p.pricing_config_id=base_config),'[]');
  END IF;
  IF legacy->>'image_price_1k' IS NOT NULL OR legacy->>'image_price_2k' IS NOT NULL OR legacy->>'image_price_4k' IS NOT NULL THEN
   image_models:=CASE WHEN legacy->>'platform'='gemini' THEN '["gemini-2.5-flash-image*","gemini-3-pro-image*","gemini-3.1-flash-image*"]'::jsonb ELSE '["gpt-image-*"]'::jsonb END;
   cards:=cards || jsonb_build_array(jsonb_build_object('models',image_models,'billing_mode','image',
    'per_request_price',COALESCE(legacy->'image_price_2k',legacy->'image_price_1k',legacy->'image_price_4k'),
    'intervals',jsonb_build_array(
     jsonb_build_object('tier_label','1K','min_tokens',0,'sort_order',0,'per_request_price',legacy->'image_price_1k'),
     jsonb_build_object('tier_label','2K','min_tokens',0,'sort_order',1,'per_request_price',legacy->'image_price_2k'),
     jsonb_build_object('tier_label','4K','min_tokens',0,'sort_order',2,'per_request_price',legacy->'image_price_4k'))));
  END IF;
  FOR card IN SELECT value FROM jsonb_array_elements(cards) LOOP
   row_id:=nextval(pg_get_serial_sequence('pricing_config_model_pricing','id'));
   card:=card || jsonb_build_object('id',row_id,'pricing_config_id',config_id,'created_at',NOW(),'updated_at',NOW());
   INSERT INTO pricing_config_model_pricing SELECT * FROM jsonb_populate_record(NULL::pricing_config_model_pricing,card);
   FOR iv IN SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(card->'intervals','null'),'[]')) LOOP
    iv:=iv || jsonb_build_object('id',nextval(pg_get_serial_sequence('pricing_config_pricing_intervals','id')),'pricing_id',row_id,'created_at',NOW(),'updated_at',NOW());
    INSERT INTO pricing_config_pricing_intervals SELECT * FROM jsonb_populate_record(NULL::pricing_config_pricing_intervals,iv);
   END LOOP;
  END LOOP;
  DELETE FROM pricing_config_groups WHERE group_id=g.id;
  INSERT INTO pricing_config_groups(pricing_config_id,group_id,created_at) VALUES(config_id,g.id,NOW());
 END LOOP;
END $$;

ALTER TABLE groups
 DROP COLUMN IF EXISTS peak_rate_enabled,
 DROP COLUMN IF EXISTS peak_start,
 DROP COLUMN IF EXISTS peak_end,
 DROP COLUMN IF EXISTS peak_rate_multiplier,
 DROP COLUMN IF EXISTS long_context_pricing_enabled,
 DROP COLUMN IF EXISTS free_openai_fast,
 DROP COLUMN IF EXISTS batch_image_discount_multiplier,
 DROP COLUMN IF EXISTS batch_image_hold_multiplier,
 DROP COLUMN IF EXISTS web_search_price_per_call,
 DROP COLUMN IF EXISTS search_price_per_1k,
 DROP COLUMN IF EXISTS audio_realtime_price_per_min,
 DROP COLUMN IF EXISTS audio_tts_price_per_million_chars,
 DROP COLUMN IF EXISTS audio_stt_price_per_hour,
 DROP COLUMN IF EXISTS model_pricing;
