import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProviderBulkActionsBar from '../ProviderBulkActionsBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('ProviderBulkActionsBar', () => {
  it('renders nothing before any row is selected', () => {
    const wrapper = mount(ProviderBulkActionsBar, {
      props: {
        selectedIds: [],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('allows selecting all results once some rows are selected', async () => {
    const wrapper = mount(ProviderBulkActionsBar, {
      props: {
        selectedIds: [1, 2],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    const button = wrapper.findAll('button').find((item) =>
      item.text().includes('admin.providers.bulkActions.selectAllResults')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('select-all-results')).toHaveLength(1)
  })

  it('emits refresh-token for selected providers', async () => {
    const wrapper = mount(ProviderBulkActionsBar, {
      attachTo: document.body,
      props: {
        selectedIds: [44],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    // 刷新令牌收在「更多」菜单里，需要先展开菜单；菜单挂载到 body。
    await wrapper.findAll('button').find((button) => button.text() === 'admin.providers.bulkActions.more')!.trigger('click')
    const item = Array.from(document.body.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')).find(
      (button) => button.textContent?.trim() === 'admin.providers.bulkActions.refreshToken'
    )
    expect(item).toBeDefined()
    item!.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('refresh-token')).toHaveLength(1)
    expect(document.body.querySelector('[role="menu"]')).toBeNull()
    expect(wrapper.text()).not.toContain('admin.providers.bulkActions.probeUpstreamBilling')
    wrapper.unmount()
  })
})
