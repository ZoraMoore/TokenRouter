import { defineComponent, type VNode } from 'vue'

/** 退出时保留上一帧的 slot，防止调用方清空结果后折叠高度突然归零。 */
export default defineComponent({
  name: 'RetainedContent',
  props: { freeze: Boolean },
  setup(props, { slots }) {
    let content: VNode[] = []
    return () => {
      if (!props.freeze) content = slots.default?.() ?? []
      return content
    }
  },
})
