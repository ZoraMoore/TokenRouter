-- 完成的绘图回答按原文保存，HTML 包装页中的样式和脚本用于作品复核与预览。
-- 历史记录的原文未知，使用空值表示。回退应用时可以保留此列。
ALTER TABLE pelican_test_results
    ADD COLUMN IF NOT EXISTS raw_answer text NOT NULL DEFAULT ''
        CHECK (octet_length(raw_answer) <= 2097152);
