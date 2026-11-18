import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import Collapse from '../Collapse.vue'
import { finishMotion, mockMotionEnvironment, nextMotionFrame } from '@/__tests__/helpers/motion'

let wrapper: VueWrapper | undefined
beforeEach(() => mockMotionEnvironment())
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
})

describe('Collapse', () => {
  it('保留草稿，关闭时立即隔离交互，退出完成后隐藏', async () => {
    const Draft = defineComponent({ setup: () => ({ value: ref('') }), template: '<input v-model="value" />' })
    wrapper = mount(Collapse, { props: { open: true }, slots: { default: Draft }, global: { stubs: { transition: false } } })
    await wrapper.get('input').setValue('draft')
    await wrapper.setProps({ open: false })
    expect(wrapper.attributes('inert')).toBeDefined()
    expect(wrapper.get('input').exists()).toBe(true)
    expect(wrapper.emitted('after-leave')).toBeUndefined()
    await finishMotion(wrapper.element)
    expect((wrapper.element as HTMLElement).style.display).toBe('none')
    expect(wrapper.emitted('after-leave')).toHaveLength(1)
    await wrapper.setProps({ open: true })
    await finishMotion(wrapper.element)
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('draft')
    expect(wrapper.attributes('inert')).toBeUndefined()
    expect(wrapper.classes()).not.toContain('motion-collapse-moving')
  })

  it('按需内容在退出完成后卸载，快速重开不会被旧完成事件清除', async () => {
    wrapper = mount(Collapse, { props: { open: false, unmountOnHide: true }, slots: { default: '<input />' }, global: { stubs: { transition: false } } })
    expect(wrapper.find('input').exists()).toBe(false)
    await wrapper.setProps({ open: true })
    await finishMotion(wrapper.element)
    await wrapper.setProps({ open: false })
    await nextMotionFrame()
    await wrapper.setProps({ open: true })
    await finishMotion(wrapper.element)
    expect(wrapper.find('input').exists()).toBe(true)
    expect(wrapper.emitted('after-leave')).toBeUndefined()
    await wrapper.setProps({ open: false })
    await finishMotion(wrapper.element)
    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('清空业务数据时保留退出内容，重新打开后采用最新内容', async () => {
    const Host = defineComponent({
      components: { Collapse },
      props: { open: Boolean, text: String },
      template: '<Collapse :open="open" unmount-on-hide><p>{{ text }}</p></Collapse>',
    })
    wrapper = mount(Host, { props: { open: true, text: 'result' }, global: { stubs: { transition: false } } })
    await wrapper.setProps({ open: false, text: '' })
    expect(wrapper.get('p').text()).toBe('result')
    await finishMotion(wrapper.element)
    await wrapper.setProps({ open: true, text: 'new result' })
    await finishMotion(wrapper.element)
    expect(wrapper.get('p').text()).toBe('new result')
    expect((wrapper.element as HTMLElement).style.height).toBe('')
  })

  it('减少动态效果不等待高度过渡', async () => {
    vi.restoreAllMocks()
    mockMotionEnvironment(true)
    wrapper = mount(Collapse, { props: { open: true, unmountOnHide: true }, slots: { default: '<input />' }, global: { stubs: { transition: false } } })
    await wrapper.setProps({ open: false })
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.emitted('after-leave')).toHaveLength(1)
  })

  it('校验定位在同一次 DOM 更新中展开保留的字段', async () => {
    const Host = defineComponent({
      components: { Collapse },
      setup: () => ({ open: ref(false) }),
      template: '<div @form-field-reveal="open = true"><Collapse :open="open"><input required /></Collapse></div>',
    })
    wrapper = mount(Host, { global: { stubs: { transition: false } } })
    await wrapper.get('input').trigger('form-field-reveal')
    const collapse = wrapper.getComponent(Collapse)
    expect((collapse.element as HTMLElement).style.display).not.toBe('none')
    expect(collapse.classes()).not.toContain('motion-collapse-moving')
    expect(collapse.attributes('inert')).toBeUndefined()
    expect(collapse.emitted('after-enter')).toHaveLength(1)
  })

  it('退出中的按需字段不参与原生校验，重新打开恢复可编辑状态', async () => {
    wrapper = mount(Collapse, {
      props: { open: true, unmountOnHide: true },
      slots: { default: '<input required /><input disabled />' },
      global: { stubs: { transition: false } },
    })
    await wrapper.setProps({ open: false })
    expect((wrapper.get('input').element as HTMLInputElement).willValidate).toBe(false)
    await wrapper.setProps({ open: true })
    await finishMotion(wrapper.element)
    const inputs = wrapper.findAll('input')
    expect((inputs[0].element as HTMLInputElement).disabled).toBe(false)
    expect((inputs[1].element as HTMLInputElement).disabled).toBe(true)
  })

  it('卸载时移除媒体查询监听且不再调用完成事件', async () => {
    vi.restoreAllMocks()
    const environment = mockMotionEnvironment()
    wrapper = mount(Collapse, { props: { open: true }, global: { stubs: { transition: false } } })
    await wrapper.setProps({ open: false })
    wrapper.unmount()
    expect(environment.listeners.size).toBe(0)
    wrapper = undefined
  })
})
