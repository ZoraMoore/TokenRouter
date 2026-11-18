import { describe, expect, it } from 'vitest'
import { reactive, ref } from 'vue'
import { useDirtyTracker } from '@/composables/useDirtyTracker'

describe('useDirtyTracker', () => {
  it('treats sources without a snapshot as clean', () => {
    const form = reactive({ name: 'a' })
    const { dirty } = useDirtyTracker({ form: () => form })

    form.name = 'b'

    expect(dirty.value).toBe(false)
  })

  it('reports changes against the snapshot and clears after markClean', () => {
    const form = reactive({ name: 'a', tags: ['x'] })
    const { dirty, markClean } = useDirtyTracker({ form: () => form })
    markClean()

    form.tags.push('y')
    expect(dirty.value).toBe(true)

    form.tags.pop()
    expect(dirty.value).toBe(false)

    form.name = 'b'
    markClean()
    expect(dirty.value).toBe(false)
  })

  it('keeps edits on one source when another source records its snapshot', () => {
    const form = reactive({ name: 'a' })
    const extra = ref(1)
    const { dirty, markClean } = useDirtyTracker({
      form: () => form,
      extra: () => extra.value,
    })
    markClean('form')

    form.name = 'b'
    extra.value = 2
    markClean('extra')

    expect(dirty.value).toBe(true)
  })
})
