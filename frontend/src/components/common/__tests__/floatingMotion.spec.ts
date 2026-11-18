import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import Collapse from '../Collapse.vue'
import Select from '../Select.vue'
import HelpTooltip from '../HelpTooltip.vue'
import { finishMotion, mockMotionEnvironment } from '@/__tests__/helpers/motion'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let wrapper: VueWrapper | undefined
beforeEach(() => mockMotionEnvironment())
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('折叠区域内的浮层', () => {
  it('父区程序化收起时同步关闭 teleported 选择框，重开保留选值', async () => {
    const Host = defineComponent({
      components: { Collapse, AppSelect: Select },
      props: { open: Boolean },
      setup: () => ({ value: ref('a'), options: [{ value: 'a', label: '选项 A' }] }),
      template: '<Collapse :open="open"><AppSelect v-model="value" :options="options" /></Collapse>',
    })
    wrapper = mount(Host, { attachTo: document.body, props: { open: true }, global: { stubs: { transition: false, Icon: true } } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    const panel = document.body.querySelector('[role="listbox"]')!
    expect(panel).not.toBeNull()
    await wrapper.setProps({ open: false })
    await flushPromises()
    expect(panel.hasAttribute('inert')).toBe(true)
    await finishMotion(panel)
    expect(document.body.querySelector('[role="listbox"]')).toBeNull()
    await wrapper.setProps({ open: true })
    await finishMotion(wrapper.element)
    expect(wrapper.get('button').text()).toContain('选项 A')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[role="listbox"]')).not.toBeNull()
  })

  it('受控提示即使尚未收到外部关闭值，也不会留在隐藏父区之外', async () => {
    const Host = defineComponent({
      components: { Collapse, HelpTooltip },
      props: { open: Boolean },
      template: '<Collapse :open="open"><HelpTooltip trigger="manual" :open="true" content="提示内容" /></Collapse>',
    })
    wrapper = mount(Host, { attachTo: document.body, props: { open: true }, global: { stubs: { transition: false, Icon: true } } })
    await flushPromises()
    const tooltip = document.body.querySelector<HTMLElement>('[role="tooltip"]')!
    expect(tooltip.style.display).not.toBe('none')
    await wrapper.setProps({ open: false })
    await flushPromises()
    expect(tooltip.hasAttribute('inert')).toBe(true)
    await finishMotion(tooltip)
    expect(tooltip.style.display).toBe('none')
  })
})
