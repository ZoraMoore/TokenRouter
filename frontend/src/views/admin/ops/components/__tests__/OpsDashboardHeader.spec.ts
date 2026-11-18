import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import zhOps from '@/i18n/locales/zh/admin/ops'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params: Record<string, string> = {}) => {
        const name = key.replace('admin.ops.', '') as keyof typeof zhOps.ops
        const message = zhOps.ops[name]
        if (typeof message !== 'string') return key
        return message.replace(/\{(\w+)\}/g, (_, field: string) => params[field] ?? '')
      },
    }),
  }
})

vi.mock('@/stores', () => ({ useAdminSettingsStore: () => ({ opsRealtimeMonitoringEnabled: false }) }))

const disks = {
  total_mb: 174080, used_mb: 40960, available_mb: 133120,
  disks: [
    { role: 'system' as const, mount_point: '/', total_mb: 73728, used_mb: 21504, available_mb: 52224, usage_percent: 29.2 },
    { role: 'data' as const, mount_point: '/mnt/data', total_mb: 100352, used_mb: 19456, available_mb: 80896, usage_percent: 19.4 },
  ],
}

function render(fullscreen = false, includeDisks = true) {
  return mount(OpsDashboardHeader, {
    props: {
      platform: '', groupId: null, timeRange: '1h', loading: false, lastUpdated: null, fullscreen,
      overview: {
        sla: 100, qps: {}, tps: {}, duration: {}, ttft: {},
        system_metrics: { disk_total_mb: 73728, disk_used_mb: 21504, disk_usage_percent: 29.2 },
        ...(includeDisks ? { disks } : {}),
      } as any,
    },
    global: {
      stubs: { Select: true, FilterDropdown: true, FilterField: true, HelpTooltip: true, BaseDialog: true, Icon: true },
    },
  })
}

describe('服务器磁盘容量', () => {
  it.each([false, true])('显示合计及两块盘明细，fullscreen=%s', fullscreen => {
    const wrapper = render(fullscreen)
    const card = wrapper.get('[data-testid="ops-disk-capacity"]').text()
    expect(card).toContain('170')
    expect(card).toContain('系统盘')
    expect(card).toContain('数据盘')
    expect(card).toContain('72')
    expect(card).toContain('98')
    expect(card).toContain('40')
    wrapper.unmount()
  })

  it('明细采集失败时标明根分区快照', () => {
    const wrapper = render(false, false)
    const card = wrapper.get('[data-testid="ops-disk-capacity"]').text()
    expect(card).toContain('磁盘明细暂不可用')
    expect(card).toContain('29.2%')
    wrapper.unmount()
  })
})
