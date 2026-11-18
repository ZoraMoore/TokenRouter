import { describe, expect, it } from 'vitest'

import { isolateLeavingElement, restoreEnteringElement } from '../leavingElement'

// 构造一个同时包含合法控件、必填空控件和按钮的退出节点。
const createPanel = () => {
  const panel = document.createElement('div')
  panel.innerHTML = `
    <input data-test="valid" value="ok" />
    <input data-test="required" required />
    <button data-test="trigger" type="button">select</button>
  `
  const get = (name: string) => panel.querySelector<HTMLInputElement | HTMLButtonElement>(`[data-test="${name}"]`)!
  return { panel, get }
}

describe('isolateLeavingElement', () => {
  it('退出时只禁用校验不通过的控件，其余控件保持可用态外观', () => {
    const { panel, get } = createPanel()
    isolateLeavingElement(panel)

    expect(panel.hasAttribute('inert')).toBe(true)
    expect(get('required').disabled).toBe(true)
    expect(get('valid').disabled).toBe(false)
    expect(get('trigger').disabled).toBe(false)
  })

  it('重新展开时恢复被临时禁用的控件', () => {
    const { panel, get } = createPanel()
    isolateLeavingElement(panel)
    restoreEnteringElement(panel)

    expect(panel.hasAttribute('inert')).toBe(false)
    expect(get('required').disabled).toBe(false)
  })
})
