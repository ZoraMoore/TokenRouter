import type { DOMWrapper } from '@vue/test-utils'

// 设置表单的布尔项统一渲染为 role="switch" 按钮，测试通过 aria-checked 读取并点击切换。
export function isSwitchOn(target: DOMWrapper<Element>): boolean {
  return target.attributes('aria-checked') === 'true'
}

export async function setSwitch(target: DOMWrapper<Element>, value: boolean): Promise<void> {
  if (isSwitchOn(target) !== value) await target.trigger('click')
}
