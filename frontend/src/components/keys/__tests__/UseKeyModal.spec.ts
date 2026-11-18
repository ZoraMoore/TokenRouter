import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { Group } from '@/types'
import UseKeyModal from '../UseKeyModal.vue'
import Select from '@/components/common/Select.vue'

const { copy } = vi.hoisted(() => ({ copy: vi.fn().mockResolvedValue(true) }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: copy }) }))
const mixedGroup = {
  id: 7, name: 'Mixed', allowed_protocols: ['anthropic_messages', 'openai_responses', 'openai_responses_websocket', 'gemini_generate_content'],
  models: ['claude-custom', 'gpt-custom', 'gemini-custom'],
  model_protocols: { 'claude-custom': ['anthropic_messages'], 'gpt-custom': ['openai_responses', 'openai_responses_websocket'], 'gemini-custom': ['gemini_generate_content'] },
} as Group
const create = (group: Group | null = mixedGroup) => mount(UseKeyModal, {
  props: { show: true, apiKey: 'sk-example', baseUrl: 'https://api.example.com/v1', group },
  global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Select: true, Toggle: true } },
})
describe('按客户端能力配置 Key', () => {
  it('未绑定分组时提示选组，不生成配置', () => {
    const wrapper = create(null)
    expect(wrapper.text()).toContain('keys.useKeyModal.noGroupDescription')
    expect(wrapper.find('pre').exists()).toBe(false)
  })
  it('混合分组显示多个客户端并按协议筛选真实模型', async () => {
    const wrapper = create()
    expect(wrapper.findAll('[role="tab"]').map(tab => tab.text())).toEqual(['Claude Code', 'Codex CLI', 'Gemini CLI', 'Grok CLI', 'OpenCode'])
    expect(wrapper.findComponent(Select).props('options')).toEqual([{ value: 'claude-custom', label: 'claude-custom' }])
    expect(wrapper.get('pre').text()).toContain('claude-custom')
    await wrapper.findAll('[role="tab"]')[1].trigger('click')
    expect(wrapper.findComponent(Select).props('options')).toEqual([{ value: 'gpt-custom', label: 'gpt-custom' }])
    expect(wrapper.get('pre').text()).toContain('model = "gpt-custom"')
  })
  it('没有可请求模型时不写入默认示例', () => {
    const wrapper = create({ ...mixedGroup, models: [] })
    expect(wrapper.find('pre').exists()).toBe(false)
    expect(wrapper.text()).toContain('keys.useKeyModal.noModels')
  })
  it('切换分组后清除已失效的模型', async () => {
    const wrapper = create()
    await wrapper.setProps({ group: { ...mixedGroup, models: ['custom'], model_protocols: { custom: ['anthropic_messages'] } } })
    await nextTick()
    expect(wrapper.get('pre').text()).toContain("ANTHROPIC_MODEL='custom'")
    expect(wrapper.get('pre').text()).not.toContain('claude-custom')
  })
  it('仅媒体入口不提供文本客户端配置', () => {
    const wrapper = create({ ...mixedGroup, allowed_protocols: ['image_batches'] })
    expect(wrapper.find('[data-testid="no-text-protocols"]').exists()).toBe(true)
    expect(wrapper.find('pre').exists()).toBe(false)
  })
  it('复制实际生成内容', async () => {
    const wrapper = create()
    const button = wrapper.findAll('button').find(button => button.text() === 'keys.useKeyModal.copy')!
    await button.trigger('click')
    expect(copy).toHaveBeenCalledWith(wrapper.get('pre').text())
    wrapper.unmount()
  })
  it('复合 Key 示例使用可请求模型并复制带前缀的 ID', async () => {
    const wrapper = create(null)
    await wrapper.setProps({ compositeGroups: [{ group_id: 7, prefix: 'shared', group: mixedGroup }] })
    expect(wrapper.text()).toContain('shared/claude-custom')
    await wrapper.get('button').trigger('click')
    expect(copy).toHaveBeenCalledWith('shared/claude-custom')
    wrapper.unmount()
  })
})
