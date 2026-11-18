import defaults from '../../../backend/internal/provider/openai_defaults.json'

// 创建表单和后端导入共用本站的模型与测试预设。
export const DEFAULT_CODEX_MODELS: readonly string[] = defaults.models
export const DEFAULT_PROVIDER_TEST_PROMPT = defaults.test_prompt
export const PREFERRED_PROVIDER_TEST_MODELS: readonly string[] = defaults.test_models

export function defaultProviderTestModel(models: { id: string }[]): string {
  return PREFERRED_PROVIDER_TEST_MODELS.find(id => models.some(model => model.id === id)) ?? ''
}
