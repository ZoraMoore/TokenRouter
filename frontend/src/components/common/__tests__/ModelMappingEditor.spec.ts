import { afterEach, describe, expect, it, vi } from 'vitest'
import { toRaw } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'

import ModelMappingEditor from '../ModelMappingEditor.vue'
import type { ModelMappingRow } from '@/utils/modelMappingRules'

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

const mountEditor = (rows: ModelMappingRow[], props: Record<string, unknown> = {}) => {
  wrapper = mount(ModelMappingEditor, {
    props: {
      modelValue: rows,
      sourceLabel: '来源模型',
      targetLabel: '目标模型',
      sourcePlaceholder: '例如 a',
      targetPlaceholder: '例如 b',
      testId: 'mapping',
      ...props,
    },
    global: {
      stubs: {
        Icon: { props: ['name'], template: '<i :data-icon="name" />' },
        'transition-group': true,
      },
    },
  })
  return wrapper
}

const lastEmittedRows = () => {
  const emitted = wrapper!.emitted('update:modelValue') ?? []
  return emitted[emitted.length - 1]?.[0] as ModelMappingRow[]
}

describe('ModelMappingEditor', () => {
  it('透传占位、无障碍标签和测试钩子', () => {
    mountEditor([{ from: 'a', to: 'b' }])

    const source = wrapper!.get('[data-testid="mapping-source-0"]')
    const target = wrapper!.get('[data-testid="mapping-target-0"]')
    expect(source.attributes('placeholder')).toBe('例如 a')
    expect(source.attributes('aria-label')).toBe('来源模型')
    expect(target.attributes('aria-label')).toBe('目标模型')
    expect((source.element as HTMLInputElement).value).toBe('a')
    expect(source.classes()).toContain('font-mono')
  })

  it('添加时发出新数组和 add 事件', async () => {
    const rows = [{ from: 'a', to: 'b' }]
    mountEditor(rows)

    await wrapper!.get('[data-testid="mapping-add"]').trigger('click')

    const next = lastEmittedRows()
    expect(next).not.toBe(rows)
    expect(next).toHaveLength(2)
    expect(toRaw(next[0])).toBe(rows[0])
    expect(next[1]).toEqual({ from: '', to: '' })
    expect(wrapper!.emitted('add')?.[0]?.[0]).toBe(next[1])
  })

  it('删除时保留其余行对象并发出 remove 事件', async () => {
    const rows = [
      { from: 'a', to: 'b' },
      { from: 'c', to: 'd' },
    ]
    mountEditor(rows)

    await wrapper!.get('[data-testid="mapping-remove-0"]').trigger('click')

    const next = lastEmittedRows()
    expect(next).toHaveLength(1)
    expect(toRaw(next[0])).toBe(rows[1])
    const removed = wrapper!.emitted('remove')?.[0]
    expect(toRaw(removed?.[0])).toBe(rows[0])
    expect(removed?.[1]).toBe(0)
  })

  it('输入时原地修改行并发出新数组，不触发增删事件', async () => {
    const rows = [{ from: 'a', to: 'b' }]
    mountEditor(rows)

    await wrapper!.get('[data-testid="mapping-target-0"]').setValue('gpt-5')

    expect(rows[0].to).toBe('gpt-5')
    expect(toRaw(lastEmittedRows()[0])).toBe(rows[0])
    expect(wrapper!.emitted('add')).toBeUndefined()
    expect(wrapper!.emitted('remove')).toBeUndefined()
  })

  it('按下标显示逐字段错误', () => {
    mountEditor(
      [
        { from: 'a', to: 'b' },
        { from: 'x*y', to: 'z' },
      ],
      { fieldErrors: [undefined, { from: '通配符无效' }] },
    )

    expect(wrapper!.get('[data-testid="mapping-source-0"]').classes()).not.toContain('input-error')
    const source = wrapper!.get('[data-testid="mapping-source-1"]')
    expect(source.classes()).toContain('input-error')
    expect(source.attributes('aria-invalid')).toBe('true')
    expect(wrapper!.get('[role="alert"]').text()).toBe('通配符无效')
  })

  it('达到上限时禁用添加', () => {
    mountEditor([{ from: 'a', to: 'b' }], { max: 1 })
    expect(wrapper!.get('[data-testid="mapping-add"]').attributes('disabled')).toBeDefined()
  })

  it('渲染 footer 插槽', () => {
    wrapper = mount(ModelMappingEditor, {
      props: { modelValue: [], sourceLabel: 's', targetLabel: 't' },
      slots: { footer: '<div data-testid="presets" />' },
      global: { stubs: { Icon: true } },
    })
    expect(wrapper.find('[data-testid="presets"]').exists()).toBe(true)
  })
})
