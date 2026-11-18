import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import zhOps from '@/i18n/locales/zh/admin/ops'
import OpsClusterResources from '../OpsClusterResources.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params: Record<string, string | number> = {}) => {
      const message = zhOps.ops[key.replace('admin.ops.', '') as keyof typeof zhOps.ops]
      if (typeof message !== 'string') return key
      return message.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? ''))
    },
  }),
}))

const usage = {
  host_cpu_cores: 8, host_cpu_usage_percent: 25,
  host_memory_total_mb: 15360, host_memory_used_mb: 4096,
  cpu_cores: 7.6, cpu_usage_percent: 10,
  memory_total_mb: 11264, memory_used_mb: 2048,
  disks: { total_mb: 250880, used_mb: 51200, available_mb: 199680, disks: [] },
}

function render(online = true) {
  return mount(OpsClusterResources, {
    props: {
      cluster: {
        total_nodes: 3, online_nodes: online ? 3 : 2, complete: online, summary: usage,
        nodes: ['15.204.66.161', '135.148.26.72', '51.81.92.195'].map((name, i) => ({
          ...usage, node_id: `node-${i}`, name, online: online || i !== 2,
          sampled_at: '2026-11-18T00:00:00Z',
        })),
      },
    },
  })
}

describe('三节点资源容量', () => {
  it('区分宿主机容量与应用限额，并显示每台服务器', () => {
    const wrapper = render()
    expect(wrapper.text()).toContain('3 / 3')
    expect(wrapper.text()).toContain('15.204.66.161')
    expect(wrapper.text()).toContain('135.148.26.72')
    expect(wrapper.text()).toContain('51.81.92.195')
    expect(wrapper.get('[data-testid="cluster-host-memory"]').text()).toContain('15')
    expect(wrapper.get('[data-testid="cluster-app-memory"]').text()).toContain('11')
    wrapper.unmount()
  })

  it('离线节点的旧使用量不作为实时指标展示', () => {
    const wrapper = render(false)
    expect(wrapper.text()).toContain('2 / 3')
    const offline = wrapper.get('[data-node-id="node-2"]')
    expect(offline.text()).toContain('离线')
    expect(offline.text()).toContain('最后采样')
    expect(offline.text()).not.toContain('25.0%')
    wrapper.unmount()
  })
})
