import { describe, expect, it } from 'vitest'
import { buildOpsPlatformOptions } from '../platformOptions'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'

describe('buildOpsPlatformOptions', () => {
  it('执行平台目录独立于分组', () => {
    expect(buildOpsPlatformOptions('全部')).toEqual([{ value: '', label: '全部' }, ...CONCRETE_PLATFORM_OPTIONS])
  })
})
