import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'

import RuleListEditor from '../RuleListEditor.vue'
import { finishMotion, mockMotionEnvironment } from '@/__tests__/helpers/motion'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

interface Row {
  name: string
}

const iconStub = { props: ['name'], template: '<i :data-icon="name" />' }

let wrapper: VueWrapper | undefined
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
})

const mountEditor = (props: Record<string, unknown>) => {
  wrapper = mount(RuleListEditor, {
    props: { testId: 'rules', ...props },
    slots: {
      row: `<template #row="{ item, index }"><input :data-testid="'field-' + index" :value="item.name" /></template>`,
    },
    global: { stubs: { Icon: iconStub, 'transition-group': true } },
  })
  return wrapper
}

// Host 模拟父级持有数据：添加时追加行，删除时移除行。
const Host = defineComponent({
  components: { RuleListEditor },
  props: { initial: { type: Array, default: () => [] } },
  setup(props) {
    const items = ref<Row[]>([...(props.initial as Row[])])
    const add = () => items.value.push({ name: '' })
    const remove = (index: number) => items.value.splice(index, 1)
    const appendSilently = () => items.value.push({ name: 'preset' })
    return { items, add, remove, appendSilently }
  },
  template: `
    <RuleListEditor :items="items" test-id="rules" title="规则" @add="add" @remove="remove">
      <template #row="{ item }"><input v-model="item.name" /></template>
    </RuleListEditor>
  `,
})

describe('RuleListEditor', () => {
  it('头部显示标题、提示和添加按钮', () => {
    mountEditor({ items: [], title: '模型重定向', hint: '说明', addLabel: '添加规则' })

    expect(wrapper!.text()).toContain('模型重定向')
    expect(wrapper!.text()).toContain('说明')
    expect(wrapper!.get('[data-testid="rules-add"]').text()).toContain('添加规则')
  })

  it('未传添加文案时使用通用文案', () => {
    mountEditor({ items: [] })
    expect(wrapper!.get('[data-testid="rules-add"]').text()).toContain('common.add')
  })

  it('footer 模式把添加按钮放在列表之后', () => {
    mountEditor({ items: [{ name: 'a' }], addPlacement: 'footer' })

    const html = wrapper!.html()
    expect(html.indexOf('rules-row')).toBeLessThan(html.indexOf('rules-add'))
  })

  it('达到上限时禁用添加', () => {
    mountEditor({ items: [{ name: 'a' }, { name: 'b' }], max: 2 })
    expect(wrapper!.get('[data-testid="rules-add"]').attributes('disabled')).toBeDefined()
  })

  it('addDisabled 单独禁用添加', () => {
    mountEditor({ items: [], addDisabled: true })
    expect(wrapper!.get('[data-testid="rules-add"]').attributes('disabled')).toBeDefined()
  })

  it('只在无行且提供空态文案时显示空态', async () => {
    mountEditor({ items: [], emptyText: '暂无规则' })
    expect(wrapper!.text()).toContain('暂无规则')

    await wrapper!.setProps({ items: [{ name: 'a' }] })
    expect(wrapper!.text()).not.toContain('暂无规则')

    await wrapper!.setProps({ items: [], emptyText: undefined })
    expect(wrapper!.find('.border-dashed').exists()).toBe(false)
  })

  it('删除和移动按下标发出事件', async () => {
    mountEditor({ items: [{ name: 'a' }, { name: 'b' }], reorderable: true })

    await wrapper!.get('[data-testid="rules-remove-1"]').trigger('click')
    await wrapper!.get('[data-testid="rules-move-down-0"]').trigger('click')
    await wrapper!.get('[data-testid="rules-move-up-1"]').trigger('click')

    expect(wrapper!.emitted('remove')).toEqual([[1]])
    expect(wrapper!.emitted('move')).toEqual([
      [0, 1],
      [1, 0],
    ])
  })

  it('首行不能上移，末行不能下移', () => {
    mountEditor({ items: [{ name: 'a' }, { name: 'b' }], reorderable: true })

    expect(wrapper!.get('[data-testid="rules-move-up-0"]').attributes('disabled')).toBeDefined()
    expect(wrapper!.get('[data-testid="rules-move-down-1"]').attributes('disabled')).toBeDefined()
    expect(wrapper!.get('[data-testid="rules-move-down-0"]').attributes('disabled')).toBeUndefined()
  })

  it('行数不超过下限时禁用删除', () => {
    mountEditor({ items: [{ name: 'a' }], min: 1 })
    expect(wrapper!.get('[data-testid="rules-remove-0"]').attributes('disabled')).toBeDefined()
  })

  it('removable 为 false 时不渲染删除按钮', () => {
    mountEditor({ items: [{ name: 'a' }], removable: false })
    expect(wrapper!.find('[data-testid="rules-remove-0"]').exists()).toBe(false)
  })

  it('卡片形态显示行序号并使用紧凑按钮', () => {
    mountEditor({
      items: [{ name: 'a' }],
      variant: 'card',
      itemLabel: (index: number) => `规则 #${index + 1}`,
    })

    const row = wrapper!.get('[data-testid="rules-row"]')
    expect(row.text()).toContain('规则 #1')
    expect(row.classes()).toContain('rounded-surface')
    expect(wrapper!.get('[data-testid="rules-remove-0"]').classes()).toContain('btn-icon-sm')
  })

  it('线形形态使用分隔线和常规按钮', () => {
    mountEditor({ items: [{ name: 'a' }] })

    expect(wrapper!.get('[data-testid="rules-row"]').classes()).toContain('border-b')
    expect(wrapper!.get('[data-testid="rules-remove-0"]').classes()).toContain('btn-icon')
  })

  it('禁用时所有操作按钮不可用', () => {
    mountEditor({ items: [{ name: 'a' }, { name: 'b' }], reorderable: true, disabled: true })

    const buttons = wrapper!.findAll('button')
    expect(buttons.length).toBeGreaterThan(0)
    expect(buttons.every((button) => button.attributes('disabled') !== undefined)).toBe(true)
  })

  it('列表级错误以 alert 渲染', () => {
    mountEditor({ items: [], error: '存在未完成的规则' })
    expect(wrapper!.get('[role="alert"]').text()).toBe('存在未完成的规则')
  })

  it('点击添加后聚焦新行的第一个输入框', async () => {
    wrapper = mount(Host, {
      props: { initial: [{ name: 'a' }] },
      attachTo: document.body,
      global: { stubs: { Icon: iconStub, 'transition-group': true } },
    })

    await wrapper.get('[data-testid="rules-add"]').trigger('click')
    await nextTick()

    const inputs = wrapper.findAll('input')
    expect(inputs).toHaveLength(2)
    expect(document.activeElement).toBe(inputs[1].element)
  })

  it('非点击添加产生的新行不抢焦点', async () => {
    wrapper = mount(Host, {
      attachTo: document.body,
      global: { stubs: { Icon: iconStub, 'transition-group': true } },
    })
    ;(wrapper.vm as unknown as { appendSilently: () => void }).appendSilently()
    await nextTick()

    expect(wrapper.findAll('input')).toHaveLength(1)
    expect(document.activeElement).toBe(document.body)
  })

  it('删除前面的行时复用后续行的节点', async () => {
    const rows = [{ name: 'a' }, { name: 'b' }]
    mountEditor({ items: rows })
    const second = wrapper!.get('[data-testid="field-1"]').element

    await wrapper!.setProps({ items: [rows[1]] })

    expect(wrapper!.get('[data-testid="field-0"]').element).toBe(second)
  })

  it('关闭动效时删除立即生效', async () => {
    wrapper = mount(RuleListEditor, {
      props: { items: [{ name: 'a' }, { name: 'b' }], animated: false, testId: 'rules' },
      slots: { row: `<template #row="{ item }"><input :value="item.name" /></template>` },
      global: { stubs: { Icon: iconStub } },
    })

    await wrapper.setProps({ items: [{ name: 'b' }] })

    expect(wrapper.findAll('input')).toHaveLength(1)
  })

  it('删除末尾的行时留在文档流中淡出，退出完成后才显示空态', async () => {
    mockMotionEnvironment()
    wrapper = mount(RuleListEditor, {
      props: { items: [{ name: 'a' }], emptyText: '暂无规则', testId: 'rules' },
      slots: { row: `<template #row="{ item }"><input :value="item.name" /></template>` },
      global: { stubs: { Icon: iconStub } },
      attachTo: document.body,
    })

    await wrapper.setProps({ items: [] })
    const leaving = wrapper.get('[data-testid="rules-row"]')
    expect(leaving.classes()).toContain('rule-list-leave-in-flow')
    expect(wrapper.text()).not.toContain('暂无规则')

    await finishMotion(leaving.element)
    expect(wrapper.find('[data-testid="rules-row"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无规则')
  })

  it('删除中间的行时后续行补位，退出行不留在文档流中', async () => {
    mockMotionEnvironment()
    const rows = [{ name: 'a' }, { name: 'b' }]
    wrapper = mount(RuleListEditor, {
      props: { items: rows, testId: 'rules' },
      slots: { row: `<template #row="{ item }"><input :value="item.name" /></template>` },
      global: { stubs: { Icon: iconStub } },
      attachTo: document.body,
    })

    await wrapper.setProps({ items: [rows[1]] })
    const leaving = wrapper.findAll('[data-testid="rules-row"]')[0]
    expect(leaving.attributes('inert')).toBeDefined()
    expect(leaving.classes()).not.toContain('rule-list-leave-in-flow')
  })
})
