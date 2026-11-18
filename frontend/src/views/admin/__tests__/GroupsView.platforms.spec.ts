import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

describe('通用分组管理', () => {
  it('不提交分组平台和默认分组字段', () => {
    const source = readFileSync('src/views/admin/GroupsView.vue', 'utf8')
    expect(source).not.toContain('createForm.platform')
    expect(source).not.toContain('editForm.platform')
    expect(source).not.toContain('is_default')
  })
})
