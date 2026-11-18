import { defineComponent, h, nextTick, ref } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useIconAnimation } from '../useIconAnimation'
import type { IconDefinition } from '../types'

const controls = vi.hoisted(() => ({
  start: vi.fn().mockResolvedValue(undefined),
  set: vi.fn(),
  stop: vi.fn()
}))
vi.mock('motion-v', () => ({ useAnimationControls: () => controls }))

const definition: IconDefinition = {
  name: 'test',
  normal: 'normal',
  animate: ['first', 'second'],
  render: () => null
}
let media: MediaQueryList
let change: (() => void) | undefined
let wrapper: VueWrapper

function host(
  options: {
    standalone?: boolean
    marked?: boolean
    disabled?: boolean
    label?: boolean
  } = {}
) {
  return mount(
    defineComponent({
      props: {
        enabled: { default: true },
        animationActive: { default: false },
        disabled: { default: false },
        spinning: { default: false }
      },
      setup(props) {
        const svg = ref<SVGSVGElement | null>(null)
        useIconAnimation(
          svg,
          () => definition,
          () => props.enabled,
          () => props.animationActive
        )
        return () => {
          const icon = h('svg', {
            ref: svg,
            class: props.spinning ? 'animate-spin' : ''
          })
          if (options.label) {
            return h('label', [h('input', { disabled: props.disabled }), icon])
          }
          return options.standalone
            ? icon
            : h(
                options.marked ? 'div' : 'button',
                {
                  disabled: props.disabled,
                  'data-icon-trigger': options.marked ? '' : undefined
                },
                [icon, h('span', '操作')]
              )
        }
      }
    }),
    { props: { disabled: options.disabled }, attachTo: document.body }
  )
}

beforeEach(() => {
  controls.start.mockReset().mockResolvedValue(undefined)
  controls.set.mockClear()
  controls.stop.mockClear()
  change = undefined
  media = {
    matches: false,
    addEventListener: vi.fn((_event, listener) => {
      change = listener
    }),
    removeEventListener: vi.fn()
  } as unknown as MediaQueryList
  vi.spyOn(window, 'matchMedia').mockReturnValue(media)
})
afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('图标交互', () => {
  it('业务状态独立于悬停触发一次，复位后可重播', async () => {
    wrapper = host()
    await wrapper.setProps({ enabled: false })
    await wrapper.trigger('pointerenter')
    expect(controls.start).not.toHaveBeenCalled()
    await wrapper.setProps({ animationActive: true })
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual(['first', 'second'])
    await wrapper.trigger('pointerenter')
    expect(controls.start).toHaveBeenCalledTimes(2)
    await wrapper.setProps({ animationActive: false })
    expect(controls.set).toHaveBeenLastCalledWith('normal')
    controls.start.mockClear()
    await wrapper.setProps({ animationActive: true })
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual(['first', 'second'])
  })

  it('业务触发仍遵守禁用和减少动态效果，关闭状态会取消旧序列', async () => {
    wrapper = host({ disabled: true })
    await wrapper.setProps({ enabled: false, animationActive: true })
    expect(controls.start).not.toHaveBeenCalled()
    Object.assign(media, { matches: true })
    await wrapper.setProps({ disabled: false })
    expect(controls.start).not.toHaveBeenCalled()
    let complete: (() => void) | undefined
    controls.start.mockImplementationOnce(() => new Promise<void>((resolve) => {
      complete = resolve
    }))
    Object.assign(media, { matches: false })
    change?.()
    expect(controls.start).toHaveBeenCalledWith('first')
    await wrapper.setProps({ animationActive: false })
    complete?.()
    await nextTick()
    expect(controls.start).toHaveBeenCalledTimes(1)
    expect(controls.set).toHaveBeenLastCalledWith('normal')
  })

  it('标签关联的表单控件在悬停中禁用时立即复位', async () => {
    wrapper = host({ label: true })
    await wrapper.trigger('pointerenter')
    expect(controls.start).toHaveBeenCalledWith('first')
    await wrapper.setProps({ disabled: true })
    await nextTick()
    expect(controls.set).toHaveBeenLastCalledWith('normal')
  })

  it('初次挂载静止，悬停整个控件播放一次，停留不重复', async () => {
    wrapper = host()
    expect(controls.start).not.toHaveBeenCalled()
    await wrapper.trigger('pointerenter')
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual([
      'first',
      'second'
    ])
    await wrapper.trigger('pointerenter')
    expect(controls.start).toHaveBeenCalledTimes(2)
    await wrapper.trigger('pointerleave')
    expect(controls.start).toHaveBeenLastCalledWith('normal')
  })

  it('聚焦后保持反馈，移出鼠标不覆盖键盘焦点', async () => {
    wrapper = host()
    await wrapper.trigger('pointerenter')
    await wrapper.trigger('focusin')
    await wrapper.trigger('pointerleave')
    expect(controls.start).not.toHaveBeenCalledWith('normal')
    await wrapper.trigger('focusout', {
      relatedTarget: wrapper.find('span').element
    })
    expect(controls.start).not.toHaveBeenCalledWith('normal')
    await wrapper.trigger('focusout', { relatedTarget: null })
    expect(controls.start).toHaveBeenLastCalledWith('normal')
  })

  it('SVG 获得点击焦点时将焦点交回按钮，键盘焦点继续保留在按钮上', () => {
    wrapper = host()
    const svg = wrapper.find('svg').element
    // 浏览器允许点击聚焦 tabindex=-1 的 SVG，不能只验证它是否退出 Tab 顺序。
    svg.setAttribute('tabindex', '-1')
    svg.focus()
    expect(document.activeElement).toBe(wrapper.element)
    expect(controls.start).toHaveBeenCalledWith('first')
    wrapper.element.blur()
    wrapper.element.focus()
    expect(document.activeElement).toBe(wrapper.element)
  })

  it('label 内的 SVG 获得焦点时转交给关联控件', () => {
    wrapper = host({ label: true })
    const svg = wrapper.find('svg').element
    svg.setAttribute('tabindex', '-1')
    svg.focus()
    expect(document.activeElement).toBe(wrapper.find('input').element)
  })

  it.each([{ standalone: true }, { marked: true }, { label: true, disabled: true }])(
    '外层无法接收焦点时清除 SVG 焦点 %o',
    (options) => {
      wrapper = host(options)
      const svg = wrapper.find('svg').element
      svg.setAttribute('tabindex', '-1')
      svg.focus()
      expect(document.activeElement).toBe(document.body)
      expect(controls.start).not.toHaveBeenCalled()
    }
  )

  it('清除内部图形的点击焦点后，仍能悬停、点击和正常复位', async () => {
    wrapper = host({ marked: true })
    const svg = wrapper.find('svg').element
    const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle')
    circle.setAttribute('tabindex', '-1')
    svg.append(circle)
    const click = vi.fn()
    wrapper.element.addEventListener('click', click)

    await wrapper.trigger('pointerenter')
    await nextTick()
    circle.focus()
    expect(document.activeElement).toBe(document.body)
    expect(controls.start).not.toHaveBeenCalledWith('normal')

    circle.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(click).toHaveBeenCalledOnce()
    await wrapper.trigger('pointerleave')
    expect(controls.start).toHaveBeenLastCalledWith('normal')
  })

  it.each([{ standalone: true }, { marked: true }])(
    '支持独立图标和显式触发容器 %o',
    async (options) => {
      wrapper = host(options)
      await wrapper.trigger('pointerenter')
      expect(controls.start).toHaveBeenCalledWith('first')
    }
  )

  it('禁用、加载和关闭属性都立即停止动画', async () => {
    wrapper = host({ disabled: true })
    // 测试工具会跳过禁用控件的 trigger；原生指针进入仍应记录悬停。
    wrapper.element.dispatchEvent(new Event('pointerenter'))
    await nextTick()
    expect(controls.start).not.toHaveBeenCalled()
    await wrapper.setProps({ disabled: false })
    await nextTick()
    expect(controls.start).toHaveBeenCalledWith('first')
    await wrapper.setProps({ spinning: true })
    await nextTick()
    expect(controls.set).toHaveBeenLastCalledWith('normal')
    await wrapper.setProps({ spinning: false })
    await wrapper.setProps({ enabled: false })
    expect(controls.set).toHaveBeenLastCalledWith('normal')
  })

  it('减少动态效果在播放中或复位途中变化都立即生效', async () => {
    wrapper = host()
    await wrapper.trigger('pointerenter')
    Object.assign(media, { matches: true })
    change?.()
    expect(controls.set).toHaveBeenLastCalledWith('normal')
    controls.start.mockClear()
    await wrapper.trigger('pointerleave')
    await wrapper.trigger('pointerenter')
    expect(controls.start).not.toHaveBeenCalled()
    Object.assign(media, { matches: false })
    change?.()
    await wrapper.trigger('pointerleave')
    controls.set.mockClear()
    Object.assign(media, { matches: true })
    change?.()
    expect(controls.set).toHaveBeenCalledWith('normal')
  })

  it('快速移出后，旧异步序列不能继续播放下一段', async () => {
    let complete: (() => void) | undefined
    controls.start.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          complete = resolve
        })
    )
    wrapper = host()
    await wrapper.trigger('pointerenter')
    await wrapper.trigger('pointerleave')
    complete?.()
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual([
      'first',
      'normal'
    ])
  })

  it('卸载后移除事件监听并取消旧序列', async () => {
    let complete: (() => void) | undefined
    controls.start.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          complete = resolve
        })
    )
    wrapper = host()
    const button = wrapper.element
    await wrapper.trigger('pointerenter')
    wrapper.unmount()
    complete?.()
    button.dispatchEvent(new Event('pointerenter'))
    change?.()
    await nextTick()
    expect(controls.start).toHaveBeenCalledTimes(1)
    expect(controls.stop).toHaveBeenCalled()
    expect(media.removeEventListener).toHaveBeenCalledWith(
      'change',
      expect.any(Function)
    )
  })

  it('触屏不留下模拟悬停动画', async () => {
    wrapper = host()
    await wrapper.trigger('pointerenter', { pointerType: 'touch' })
    expect(controls.start).not.toHaveBeenCalled()
  })
})
