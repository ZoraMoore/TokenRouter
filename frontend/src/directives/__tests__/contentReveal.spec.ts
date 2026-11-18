import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, onMounted } from 'vue'
import { createMemoryHistory, createRouter, useRoute } from 'vue-router'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { vContentReveal } from '../contentReveal'
import { mockMotionEnvironment } from '@/__tests__/helpers/motion'

let wrapper: VueWrapper | undefined
const animations: Array<{ cancel: ReturnType<typeof vi.fn>; finished: Promise<void> }> = []
const animate = vi.fn(() => {
  const animation = { cancel: vi.fn(), finished: new Promise<void>(() => {}) }
  animations.push(animation)
  return animation
})
const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'animate')

beforeEach(() => {
  animations.length = 0
  animate.mockClear()
  Object.defineProperty(HTMLElement.prototype, 'animate', { configurable: true, value: animate })
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  if (originalAnimate) Object.defineProperty(HTMLElement.prototype, 'animate', originalAnimate)
  else delete (HTMLElement.prototype as { animate?: unknown }).animate
})

async function mountPage() {
  let mounts = 0
  const Page = defineComponent({
    directives: { contentReveal: vContentReveal },
    setup() {
      onMounted(() => mounts++)
      return { route: useRoute() }
    },
    template: '<main v-content-reveal="route.path" style="--motion-fast: 150ms; --motion-ease: ease-out"><input /></main>',
  })
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/page/:id', component: Page }] })
  await router.push('/page/1')
  wrapper = mount({ template: '<RouterView />' }, { global: { plugins: [router] } })
  await flushPromises()
  return { router, mounts: () => mounts }
}

describe('页面内容淡入', () => {
  it('查询参数及 hash 不触发动画，参数换页保留组件与输入状态', async () => {
    mockMotionEnvironment()
    const page = await mountPage()
    await wrapper!.get('input').setValue('draft')
    expect(animate).toHaveBeenCalledTimes(1)
    await page.router.push('/page/1?sort=cost#details')
    await flushPromises()
    expect(animate).toHaveBeenCalledTimes(1)
    await page.router.push('/page/2')
    await flushPromises()
    expect(animate).toHaveBeenCalledTimes(2)
    expect(animations[0].cancel).toHaveBeenCalledOnce()
    expect(page.mounts()).toBe(1)
    expect((wrapper!.get('input').element as HTMLInputElement).value).toBe('draft')
    expect(animate.mock.calls[0]).toEqual([[{ opacity: 0 }, { opacity: 1 }], { duration: 150, easing: 'ease-out' }])
  })

  it('快速换页和动态开启减少动画时取消旧动画，卸载移除监听', async () => {
    const environment = mockMotionEnvironment()
    const page = await mountPage()
    await page.router.push('/page/2')
    await page.router.push('/page/3')
    await flushPromises()
    expect(animations[0].cancel).toHaveBeenCalledOnce()
    expect(animations[1].cancel).toHaveBeenCalledOnce()
    environment.reduce(true)
    expect(animations[2].cancel).toHaveBeenCalledOnce()
    await page.router.push('/page/4')
    await flushPromises()
    expect(animate).toHaveBeenCalledTimes(3)
    wrapper!.unmount()
    wrapper = undefined
    expect(environment.listeners.size).toBe(0)
  })

  it('减少动态效果下首屏立即可用', async () => {
    mockMotionEnvironment(true)
    await mountPage()
    expect(animate).not.toHaveBeenCalled()
    expect(wrapper!.find('input').exists()).toBe(true)
  })
})
