type FormControl = HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | HTMLButtonElement
const disabledControls = new WeakMap<Element, FormControl[]>()

/**
 * 退出元素设置 inert，停止响应交互。
 * 临时禁用校验未通过的控件，让待移除的必填字段退出表单校验。
 * 其它控件保持当前状态，退出动画期间底色保持稳定。
 */
export function isolateLeavingElement(element: Element) {
  element.setAttribute('inert', '')
  if (disabledControls.has(element)) return
  const controls = Array.from(element.querySelectorAll<FormControl>('input, textarea, select, button'))
    .filter(control => !control.disabled && !control.validity.valid)
  disabledControls.set(element, controls)
  controls.forEach(control => { control.disabled = true })
}

/** 退出被新一次展开取消时，只恢复由动效暂时禁用的控件。 */
export function restoreEnteringElement(element: Element) {
  element.removeAttribute('inert')
  if (element instanceof HTMLElement) element.style.removeProperty('--motion-list-width')
  disabledControls.get(element)?.forEach(control => { control.disabled = false })
  disabledControls.delete(element)
}

/** 列表项退出并脱离文档流前固定当前宽度，使多列表单和标签保持展开尺寸。 */
export function prepareListLeave(element: Element) {
  if (element instanceof HTMLElement) {
    element.style.setProperty('--motion-list-width', `${element.getBoundingClientRect().width}px`)
  }
  isolateLeavingElement(element)
}
