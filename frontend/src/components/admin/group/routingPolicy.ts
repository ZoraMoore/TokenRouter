import type { GroupRoutingPolicy } from '@/types'

// 每个分组使用独立的模型规则草稿。
export function defaultRoutingPolicy(): GroupRoutingPolicy {
  return { enabled: true, model_mapping: {}, restrict_models: false, restriction_model_source: 'group_mapped', allowed_models: [], features: '', features_config: {} }
}

// 表单保存后应用各项设置，enabled 固定为 true，并复制已有协议字段。
export function cloneRoutingPolicy(policy?: GroupRoutingPolicy): GroupRoutingPolicy {
  const copied = policy ? JSON.parse(JSON.stringify(policy)) as GroupRoutingPolicy : defaultRoutingPolicy()
  return { ...defaultRoutingPolicy(), ...copied, enabled: true, model_mapping: copied.model_mapping ?? {}, allowed_models: copied.allowed_models ?? [], features_config: copied.features_config ?? {}, features: copied.features ?? '' }
}
