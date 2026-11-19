import { defineComponent, h, nextTick } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Icon from '../Icon.vue'
import AnimatedIconArtwork from '../AnimatedIconArtwork'
import type { IconName } from '../registry'

const controls = vi.hoisted(() => ({
  start: vi.fn().mockResolvedValue(undefined),
  stop: vi.fn(),
  subscribe: vi.fn(() => () => {})
}))
const createControls = vi.hoisted(() => vi.fn(() => controls))
vi.mock('motion-v', async (importOriginal) => {
  const motion = await importOriginal<typeof import('motion-v')>()
  return { ...motion, useAnimationControls: createControls }
})

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
        name: { default: 'layers' },
        inert: { default: false },
        spinning: { default: false }
      },
      setup(props) {
        return () => {
          const icon = h(Icon, {
            name: props.name as IconName,
            animateOnHover: props.enabled,
            animationActive: props.animationActive,
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
                  inert: props.inert || undefined,
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
  controls.stop.mockClear()
  controls.subscribe.mockClear()
  createControls.mockClear()
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
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual(['firstState', 'secondState'])
    await wrapper.trigger('pointerenter')
    expect(controls.start).toHaveBeenCalledTimes(2)
    await wrapper.setProps({ animationActive: false })
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
    controls.start.mockClear()
    await wrapper.setProps({ animationActive: true })
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual(['firstState', 'secondState'])
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
    await nextTick()
    expect(controls.start).toHaveBeenCalledWith('firstState')
    await wrapper.setProps({ animationActive: false })
    complete?.()
    await nextTick()
    expect(controls.start).toHaveBeenCalledTimes(1)
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
  })

  it('标签关联的表单控件在悬停中禁用时立即复位', async () => {
    wrapper = host({ label: true })
    await wrapper.trigger('pointerenter')
    expect(controls.start).toHaveBeenCalledWith('firstState')
    await wrapper.setProps({ disabled: true })
    await nextTick()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
  })

  it('初次挂载静止，悬停整个控件播放一次，停留不重复', async () => {
    const observe = vi.spyOn(MutationObserver.prototype, 'observe')
    wrapper = host()
    expect(createControls).not.toHaveBeenCalled()
    expect(observe).not.toHaveBeenCalled()
    expect(media.addEventListener).not.toHaveBeenCalled()
    controls.start.mockImplementationOnce(async () => {
      expect(controls.subscribe).toHaveBeenCalled()
    })
    await wrapper.trigger('pointerenter')
    await nextTick()
    expect(createControls).toHaveBeenCalledOnce()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual([
      'firstState',
      'secondState'
    ])
    await wrapper.trigger('pointerenter')
    expect(controls.start).toHaveBeenCalledTimes(2)
    await wrapper.trigger('pointerleave')
    expect(controls.start).toHaveBeenLastCalledWith('normal')
    await nextTick()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
    expect(media.removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })

  it('首次键盘聚焦播放一次，根 SVG 保持在控件内', async () => {
    wrapper = host()
    const svg = wrapper.find('svg').element
    wrapper.element.focus()
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual(['firstState', 'secondState'])
    expect(wrapper.find('svg').element).toBe(svg)
    expect(document.activeElement).toBe(wrapper.element)
  })

  it.each(['brain', 'document', 'chart', 'archive', 'sparkles'])(
    '静态图形与动画首帧的 SVG 几何相同 %s',
    async (name) => {
      wrapper = host()
      await wrapper.setProps({ name })
      const svg = wrapper.find('svg').element
      const shape = () => Array.from(svg.querySelectorAll('g, path, circle, line, rect'), (node) =>
        [node.tagName, ...['d', 'fill', 'stroke-width', 'x1', 'x2', 'cx', 'cy', 'pathLength', 'stroke-dasharray']
          .map((attr) => node.getAttribute(attr))]
      )
      const resting = shape()
      await wrapper.trigger('pointerenter')
      expect(shape()).toEqual(resting)
      expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(true)
    }
  )

  it('缩放图标在悬停时使用 CSS 动画', async () => {
    wrapper = host()
    await wrapper.setProps({ name: 'modalityText' })
    expect(wrapper.find('.icon-scale-feedback').exists()).toBe(false)
    await wrapper.trigger('pointerenter')
    expect(wrapper.find('.icon-scale-feedback').exists()).toBe(true)
    expect(createControls).not.toHaveBeenCalled()
    await wrapper.trigger('pointerleave')
    expect(wrapper.find('.icon-scale-feedback').exists()).toBe(false)
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

  it('SVG 获得点击焦点时将焦点交回按钮，键盘焦点继续保留在按钮上', async () => {
    wrapper = host()
    const svg = wrapper.find('svg').element
    // 浏览器允许点击聚焦 tabindex=-1 的 SVG，不能只验证它是否退出 Tab 顺序。
    svg.setAttribute('tabindex', '-1')
    svg.focus()
    expect(document.activeElement).toBe(wrapper.element)
    await nextTick()
    expect(controls.start).toHaveBeenCalledWith('firstState')
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
      expect(controls.start).toHaveBeenCalledWith('firstState')
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
    expect(controls.start).toHaveBeenCalledWith('firstState')
    await wrapper.setProps({ spinning: true })
    await nextTick()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
    await wrapper.setProps({ spinning: false })
    await wrapper.setProps({ enabled: false })
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
  })

  it('减少动态效果在播放中或复位途中变化都立即生效', async () => {
    wrapper = host()
    await wrapper.trigger('pointerenter')
    Object.assign(media, { matches: true })
    change?.()
    await nextTick()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
    controls.start.mockClear()
    await wrapper.trigger('pointerleave')
    await wrapper.trigger('pointerenter')
    expect(controls.start).not.toHaveBeenCalled()
    Object.assign(media, { matches: false })
    change?.()
    await nextTick()
    controls.start.mockImplementationOnce(() => new Promise<void>(() => {}))
    await wrapper.trigger('pointerleave')
    expect(controls.start).toHaveBeenLastCalledWith('normal')
    Object.assign(media, { matches: true })
    change?.()
    await nextTick()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
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
      'firstState',
      'normal'
    ])
  })

  it('第一次悬停在挂载前结束时继续显示静态图形', async () => {
    wrapper = host()
    wrapper.element.dispatchEvent(new Event('pointerenter'))
    wrapper.element.dispatchEvent(new Event('pointerleave'))
    await nextTick()
    expect(createControls).not.toHaveBeenCalled()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
  })

  it('同一轮事件内移出再进入会重播，先前的序列随即结束', async () => {
    let complete: (() => void) | undefined
    controls.start.mockImplementationOnce(() => new Promise<void>((resolve) => {
      complete = resolve
    }))
    wrapper = host()
    await wrapper.trigger('pointerenter')
    wrapper.element.dispatchEvent(new Event('pointerleave'))
    wrapper.element.dispatchEvent(new Event('pointerenter'))
    complete?.()
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual([
      'firstState', 'normal', 'firstState', 'secondState'
    ])
  })

  it('悬停中换图会取消进行中的序列并播放当前图形', async () => {
    let complete: (() => void) | undefined
    controls.start.mockImplementationOnce(() => new Promise<void>((resolve) => {
      complete = resolve
    }))
    wrapper = host()
    const svg = wrapper.find('svg').element
    await wrapper.trigger('pointerenter')
    await wrapper.setProps({ name: 'plus' })
    complete?.()
    await nextTick()
    expect(controls.start.mock.calls.map(([target]) => target)).toEqual(['firstState', 'animate'])
    expect(wrapper.find('svg').element).toBe(svg)
    expect(wrapper.find('svg').attributes('data-animated-icon')).toBe('plus')
  })

  it('控件变为 inert 时恢复静态图形，解除后响应当前悬停', async () => {
    wrapper = host()
    await wrapper.trigger('pointerenter')
    const inertParent = wrapper.element.parentElement
    inertParent?.setAttribute('inert', '')
    await nextTick()
    await nextTick()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(false)
    inertParent?.removeAttribute('inert')
    await nextTick()
    await nextTick()
    expect(wrapper.findComponent(AnimatedIconArtwork).exists()).toBe(true)
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
    const removeListener = vi.spyOn(button, 'removeEventListener')
    controls.stop.mockClear()
    wrapper.unmount()
    complete?.()
    button.dispatchEvent(new Event('pointerenter'))
    change?.()
    await nextTick()
    expect(controls.start).toHaveBeenCalledTimes(1)
    expect(controls.stop).toHaveBeenCalled()
    expect(removeListener).toHaveBeenCalledWith('pointerenter', expect.any(Function))
    expect(removeListener).toHaveBeenCalledWith('focusin', expect.any(Function))
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
