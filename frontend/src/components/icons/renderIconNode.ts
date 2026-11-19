import { h } from 'vue'
import type { RenderIconNode } from './types'

// 装饰图形退出 Tab 顺序，鼠标点击仍可传递到外层控件。
export const artworkFocusAttrs = { tabindex: -1, focusable: 'false' }

/** 直接绘制初始变体的 SVG 几何。静止时的位移、旋转和缩放为单位变换。 */
export const renderStaticIconNode: RenderIconNode = (tag, props, children) => {
  const {
    animate: _animate,
    custom: _custom,
    initial,
    transition: _transition,
    variants,
    ...attrs
  } = props
  const frame = typeof initial === 'string' ? variants?.[initial] : undefined
  if (frame && typeof frame !== 'function') {
    const { d, x1, x2, fill, strokeWidth, strokeDashoffset, pathLength } = frame
    if (d !== undefined) attrs.d = d
    if (x1 !== undefined) attrs.x1 = x1
    if (x2 !== undefined) attrs.x2 = x2
    if (fill !== undefined) attrs.fill = fill
    if (strokeWidth !== undefined) attrs['stroke-width'] = strokeWidth
    if (strokeDashoffset !== undefined) attrs['stroke-dashoffset'] = strokeDashoffset
    if (pathLength !== undefined) {
      // 逐段描线使用归一化长度，静态图形使用同样的初始虚线参数。
      attrs.pathLength = 1
      attrs['stroke-dasharray'] = `${pathLength} ${frame.pathSpacing ?? 1}`
      attrs['stroke-dashoffset'] = -Number(frame.pathOffset ?? 0)
    }
  }
  return h(
    tag,
    { ...attrs, ...artworkFocusAttrs },
    typeof children === 'function' ? children() : children
  )
}
