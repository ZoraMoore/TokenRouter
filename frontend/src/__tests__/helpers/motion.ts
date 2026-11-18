import { nextTick } from 'vue'
import { vi } from 'vitest'

/** jsdom 不计算 CSS 过渡；显式提供浏览器时长，并由 transitionend 推进完成。 */
export function mockMotionEnvironment(initiallyReduced = false) {
  let reduced = initiallyReduced
  const listeners = new Set<(event: MediaQueryListEvent) => void>()
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    media: query,
    get matches() { return query.includes('prefers-reduced-motion') && reduced },
    addEventListener: (_type: string, callback: () => void) => listeners.add(callback),
    removeEventListener: (_type: string, callback: () => void) => listeners.delete(callback),
    addListener: (callback: () => void) => listeners.add(callback),
    removeListener: (callback: () => void) => listeners.delete(callback),
    onchange: null,
    dispatchEvent: () => true,
  } as MediaQueryList))
  const originalStyle = window.getComputedStyle.bind(window)
  vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => {
    const style = originalStyle(element)
    return new Proxy(style, {
      get(target, key) {
        if (key === 'transitionDuration') return reduced ? '0s' : '0.22s'
        if (key === 'transitionDelay' || key === 'animationDuration' || key === 'animationDelay') return '0s'
        const value = Reflect.get(target, key)
        return typeof value === 'function' ? value.bind(target) : value
      },
    })
  })
  return {
    reduce(value: boolean) {
      reduced = value
      // VueUse 从 change 事件读取 matches，保持与浏览器通知的形状一致。
      listeners.forEach(callback => callback({ matches: reduced } as MediaQueryListEvent))
    },
    listeners,
  }
}

export async function nextMotionFrame() {
  await nextTick()
  await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
  await nextTick()
}

export async function finishMotion(element: Element) {
  await nextMotionFrame()
  element.dispatchEvent(new Event('transitionend', { bubbles: true }))
  await nextTick()
}
