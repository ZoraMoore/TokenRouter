import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageSparkline from '../UsageSparkline.vue'

const pathOf = (values: Array<number | null>) =>
  mount(UsageSparkline, { props: { values } }).find('path').attributes('d')

describe('UsageSparkline', () => {
  it('按最大值等比缩放，最高点贴近顶部，0 贴近底部', () => {
    expect(pathOf([0, 5, 10])).toBe('M0.00 30.00 L50.00 16.00 L100.00 2.00')
  })

  it('全为 0 时画贴底平线，只有一个时段时画成横线', () => {
    expect(pathOf([0, 0])).toBe('M0.00 30.00 L100.00 30.00')
    expect(pathOf([3])).toBe('M0 2.00 L100 2.00')
  })

  it('跳过 null 并连接前后时段，没有数据时不画线', () => {
    expect(pathOf([1, null, 1])).toBe('M0.00 2.00 L100.00 2.00')
    expect(mount(UsageSparkline, { props: { values: [null, null] } }).find('path').exists()).toBe(false)
    expect(mount(UsageSparkline, { props: { values: [] } }).find('path').exists()).toBe(false)
  })
})
