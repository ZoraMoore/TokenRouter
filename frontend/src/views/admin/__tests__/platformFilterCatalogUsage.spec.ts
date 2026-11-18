import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

function readSource(path: string): string {
  return readFileSync(resolve(path), 'utf8')
}

describe('admin platform filters', () => {
  it('does not constrain groups or subscriptions to a platform', () => {
    for (const path of ['src/views/admin/GroupsView.vue', 'src/views/admin/SubscriptionsView.vue']) {
      const source = readSource(path)
      expect(source).not.toContain('filters.platform')
    }
  })

  it('uses the concrete catalog for provider and error filters', () => {
    for (const path of [
      'src/components/admin/provider/ProviderTableFilters.vue',
      'src/components/admin/ErrorPassthroughRulesModal.vue'
    ]) {
      const source = readSource(path)
      expect(source).toContain('CONCRETE_PLATFORM_OPTIONS')
    }
  })

  it('keeps the operations helper on the shared catalog for actual provider platforms', () => {
    const source = readSource('src/views/admin/ops/platformOptions.ts')
    expect(source).toContain('CONCRETE_PLATFORM_OPTIONS')
  })
})
