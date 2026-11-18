import defaults from '../../../backend/internal/provider/openai_defaults.json'

// 连接测试和后端共用提示词及首选型号。
export const DEFAULT_PROVIDER_TEST_PROMPT = defaults.test_prompt
export const PREFERRED_PROVIDER_TEST_MODELS: readonly string[] = defaults.test_models

export function defaultProviderTestModel(models: { id: string }[]): string {
  return PREFERRED_PROVIDER_TEST_MODELS.find(id => models.some(model => model.id === id)) ?? ''
}
