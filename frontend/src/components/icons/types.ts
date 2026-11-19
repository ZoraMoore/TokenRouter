import type { VNode, VNodeChild } from 'vue'
import type { MotionProps, useAnimationControls } from 'motion-v'

export type IconControls = ReturnType<typeof useAnimationControls>
export type IconAnimation = Parameters<IconControls['start']>[0]
export type IconVariants = NonNullable<MotionProps<'g'>['variants']>

export type IconTag = 'g' | 'path' | 'circle' | 'rect' | 'line' | 'polyline' | 'polygon' | 'ellipse'

export type IconNodeProps = Record<string, unknown> &
  Pick<MotionProps<'g'>, 'custom' | 'initial' | 'style' | 'transition' | 'variants'> & {
    animate?: true
  }

// 通过同一个绘制函数提供 SVG 路径和变体。子节点延迟求值，供 Motion 继承父级变体。
export type RenderIconNode = (
  tag: IconTag,
  props: IconNodeProps,
  children?: VNodeChild[] | (() => VNodeChild[])
) => VNode

interface IconArtwork<Result extends VNodeChild = VNodeChild> {
  name: string
  render: (node: RenderIconNode, strokeWidth: number) => Result
}

export interface AnimatedIconDefinition extends IconArtwork {
  normal: string
  animate: IconAnimation[]
}

export type IconDefinition = AnimatedIconDefinition | (IconArtwork<VNode[]> & { scale: true })
