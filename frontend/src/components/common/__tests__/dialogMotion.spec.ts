import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import BaseDialog from '../BaseDialog.vue'
import AuthCardDialog from '../AuthCardDialog.vue'
import { finishMotion, mockMotionEnvironment, nextMotionFrame } from '@/__tests__/helpers/motion'

const wrappers: VueWrapper[] = []
beforeEach(() => mockMotionEnvironment())
afterEach(() => {
  wrappers.splice(0).reverse().forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

function dialog() {
  const wrapper = mount(BaseDialog, {
    attachTo: document.body,
    props: { show: true, title: '测试弹窗' },
    slots: { default: '<input required />' },
    global: { stubs: { transition: false, Icon: true } },
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('弹窗退出生命周期', () => {
  it('Escape 只关闭最上层弹窗', async () => {
    const first = dialog()
    const second = dialog()
    await nextTick()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(first.emitted('close')).toBeUndefined()
    expect(second.emitted('close')).toHaveLength(1)
  })

  it('退出中保持滚动锁，隔离操作和校验，结束后恢复原焦点', async () => {
    const trigger = document.createElement('button')
    document.body.append(trigger)
    trigger.focus()
    const wrapper = dialog()
    await nextTick()
    const overlay = document.querySelector('.modal-overlay')!
    await wrapper.setProps({ show: false })
    expect(overlay.hasAttribute('inert')).toBe(true)
    expect(overlay.querySelector('input')!.willValidate).toBe(false)
    expect(document.body.classList.contains('modal-open')).toBe(true)
    await finishMotion(overlay)
    expect(document.querySelector('.modal-overlay')).toBeNull()
    expect(document.body.classList.contains('modal-open')).toBe(false)
    expect(document.activeElement).toBe(trigger)
    expect(wrapper.emitted('after-leave')).toHaveLength(1)
  })

  it('退出途中重开保留锁和可操作内容', async () => {
    const wrapper = dialog()
    await nextTick()
    const overlay = document.querySelector('.modal-overlay')!
    await wrapper.setProps({ show: false })
    await nextMotionFrame()
    await wrapper.setProps({ show: true })
    const reopened = document.querySelector('.modal-overlay')!
    await finishMotion(reopened)
    overlay.dispatchEvent(new Event('transitionend', { bubbles: true }))
    expect(document.body.classList.contains('modal-open')).toBe(true)
    expect(reopened.hasAttribute('inert')).toBe(false)
    expect(reopened.querySelector('input')!.disabled).toBe(false)
    expect(wrapper.emitted('after-leave')).toBeUndefined()
  })

  it('旧弹窗退出不抢走新弹窗焦点，共享锁在最后一个外壳卸载时释放', async () => {
    const first = dialog()
    await nextTick()
    const overlay = document.querySelector('.modal-overlay')!
    await first.setProps({ show: false })
    const second = mount(AuthCardDialog, {
      attachTo: document.body,
      props: { show: true },
      slots: { default: '<button>新弹窗</button>' },
      global: { stubs: { transition: false } },
    })
    wrappers.push(second)
    await nextTick()
    const focus = second.get('button').element
    await finishMotion(overlay)
    expect(document.activeElement).toBe(focus)
    expect(document.body.classList.contains('modal-open')).toBe(true)
    second.unmount()
    wrappers.pop()
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })
})
