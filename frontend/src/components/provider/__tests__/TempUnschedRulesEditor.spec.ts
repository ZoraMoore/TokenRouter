import { afterEach, describe, expect, it, vi } from 'vitest'
import { toRaw } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'

import TempUnschedRulesEditor, { type TempUnschedRuleForm } from '../TempUnschedRulesEditor.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

let wrapper: VueWrapper | undefined
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

const rule = (code: number): TempUnschedRuleForm => ({
  error_code: code,
  keywords: '',
  duration_minutes: 5,
  description: '',
})

const mountEditor = (rules: TempUnschedRuleForm[]) => {
  wrapper = mount(TempUnschedRulesEditor, {
    props: { modelValue: rules },
    global: { stubs: { Icon: true, 'transition-group': true } },
  })
  return wrapper
}

const lastEmitted = () =>
  wrapper!.emitted('update:modelValue')?.at(-1)?.[0] as TempUnschedRuleForm[]

const findButton = (text: string) =>
  wrapper!.findAll('button').find((button) => button.text().includes(text))!

describe('TempUnschedRulesEditor', () => {
  it('添加空规则时使用 30 分钟默认时长', async () => {
    mountEditor([])

    await findButton('admin.providers.tempUnschedulable.addRule').trigger('click')

    expect(lastEmitted()).toEqual([
      { error_code: null, keywords: '', duration_minutes: 30, description: '' },
    ])
  })

  it('预设追加规则副本', async () => {
    mountEditor([])

    await findButton('admin.providers.tempUnschedulable.presets.rateLimitLabel').trigger('click')

    expect(lastEmitted()).toEqual([
      expect.objectContaining({ error_code: 429, duration_minutes: 10 }),
    ])
  })

  it('上下移动交换相邻规则', async () => {
    const rules = [rule(429), rule(503), rule(529)]
    mountEditor(rules)

    await wrapper!.findAll('button[aria-label="common.moveDown"]')[0].trigger('click')

    expect(lastEmitted().map((item) => toRaw(item))).toEqual([rules[1], rules[0], rules[2]])
  })

  it('删除指定规则并显示规则序号', async () => {
    const rules = [rule(429), rule(503)]
    mountEditor(rules)
    expect(wrapper!.text()).toContain('admin.providers.tempUnschedulable.ruleIndex')

    await wrapper!.findAll('button[aria-label="common.delete"]')[1].trigger('click')

    expect(lastEmitted().map((item) => toRaw(item))).toEqual([rules[0]])
  })
})
