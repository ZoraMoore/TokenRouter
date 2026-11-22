import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { setSwitch } from '@/__tests__/helpers/switches'

import RedeemView from '../RedeemView.vue'

const {
  listRedeemCodes,
  generateRedeemCodes,
  batchUpdateRedeemCodes,
  getClaimLimit,
  setClaimLimit,
  getPlans,
  showSuccess,
  showError,
  showInfo
} =
  vi.hoisted(() => ({
    listRedeemCodes: vi.fn(),
    generateRedeemCodes: vi.fn(),
    batchUpdateRedeemCodes: vi.fn(),
    getClaimLimit: vi.fn(),
    setClaimLimit: vi.fn(),
    getPlans: vi.fn(),
    showSuccess: vi.fn(),
    showError: vi.fn(),
    showInfo: vi.fn()
  }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    redeem: {
      list: listRedeemCodes,
      generate: generateRedeemCodes,
      update: vi.fn(),
      delete: vi.fn(),
      batchDelete: vi.fn(),
      batchUpdate: batchUpdateRedeemCodes,
      getClaimLimit,
      setClaimLimit,
      exportCodes: vi.fn()
    },
    payment: {
      getPlans
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError,
    showInfo
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn()
  })
}))

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({
    formatBalanceAmount: (value: number) => `$${value.toFixed(2)}`
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <table>
      <thead>
        <tr>
          <th v-for="column in columns" :key="column.key">
            <slot :name="'header-' + column.key" :column="column">{{ column.label }}</slot>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in data" :key="row.id">
          <td v-for="column in columns" :key="column.key">
            <slot :name="'cell-' + column.key" :row="row" :value="row[column.key]">
              {{ row[column.key] }}
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  `
}

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  setup(
    props: { options: Array<{ value: unknown; label: string }> },
    { emit }: { emit: (event: string, ...args: unknown[]) => void }
  ) {
    const onChange = (event: Event) => {
      const raw = (event.target as HTMLSelectElement).value
      const option = props.options.find((item) => String(item.value ?? '') === raw)
      const value = option ? option.value : raw
      emit('update:modelValue', value)
      emit('change', value, option ?? null)
    }
    return { onChange }
  },
  template: `
    <select v-bind="$attrs" :value="modelValue ?? ''" @change="onChange">
      <option v-for="option in options" :key="String(option.value ?? '')" :value="option.value ?? ''">
        {{ option.label }}
      </option>
    </select>
  `
}

describe('admin RedeemView batch update', () => {
  beforeEach(() => {
    localStorage.clear()
    document.body.innerHTML = ''

    listRedeemCodes.mockReset()
    generateRedeemCodes.mockReset()
    batchUpdateRedeemCodes.mockReset()
    getClaimLimit.mockReset()
    setClaimLimit.mockReset()
    getPlans.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    showInfo.mockReset()

    listRedeemCodes.mockResolvedValue({
      items: [
        {
          id: 1,
          code: 'CODE-1',
          type: 'balance',
          value: 10,
          status: 'unused',
          max_uses: 1,
          used_count: 0,
          used_by: null,
          used_at: null,
          created_at: '2026-01-01T00:00:00Z',
          expires_at: null
        },
        {
          id: 2,
          code: 'CODE-2',
          type: 'balance',
          value: 20,
          status: 'unused',
          max_uses: 1,
          used_count: 0,
          used_by: null,
          used_at: null,
          created_at: '2026-01-01T00:00:00Z',
          expires_at: null
        }
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    generateRedeemCodes.mockResolvedValue([
      {
        id: 3,
        code: 'CODE-3',
        type: 'balance',
        value: 10,
        status: 'unused',
        max_uses: 5,
        used_count: 0,
        used_by: null,
        used_at: null,
        created_at: '2026-01-01T00:00:00Z',
        expires_at: null
      }
    ])
    batchUpdateRedeemCodes.mockResolvedValue({ updated: 1, message: 'ok' })
    getPlans.mockResolvedValue({ data: [] })
  })

  it('保存余额和优惠券共用的限领时段并拒绝颠倒的时间', async () => {
    const settings = { enabled: false, starts_at: null, ends_at: null, tracking_since: 1 }
    getClaimLimit.mockResolvedValue(settings)
    setClaimLimit.mockImplementation(async (limit) => ({ ...settings, ...limit }))
    const wrapper = mount(RedeemView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /></div>' },
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
          Icon: true,
          Teleport: true
        }
      }
    })
    try {
      await flushPromises()
      await wrapper.get('[data-testid="claim-limit-open"]').trigger('click')
      await flushPromises()
      await setSwitch(wrapper.get('[data-testid="claim-limit-enabled"]'), true)
      const startsAt = wrapper.get('#redeem-claim-limit-starts-at')
      const endsAt = wrapper.get('#redeem-claim-limit-ends-at')
      await startsAt.setValue('2026-11-20T18:00')
      await endsAt.setValue('2026-11-20T17:00')
      await wrapper.get('#redeem-claim-limit-form').trigger('submit')
      expect(setClaimLimit).not.toHaveBeenCalled()
      expect(wrapper.text()).toContain('admin.redeem.claimLimit.invalid')

      await endsAt.setValue('2026-11-20T20:00')
      await wrapper.get('#redeem-claim-limit-form').trigger('submit')
      await flushPromises()
      expect(setClaimLimit).toHaveBeenCalledExactlyOnceWith({
        enabled: true,
        starts_at: new Date(2026, 10, 20, 18).getTime() / 1000,
        ends_at: new Date(2026, 10, 20, 20).getTime() / 1000
      })
      expect(showSuccess).toHaveBeenCalledWith('admin.redeem.claimLimit.saved')
      expect(wrapper.find('#redeem-claim-limit-form').exists()).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })

  it('submits only checked fields for selected redeem codes', async () => {
    const wrapper = mount(RedeemView, {
      attachTo: document.body,
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          Select: SelectStub,
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot /><slot name="footer" /></div>'
          },
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()
    await wrapper.findAll('[data-testid="select-code"]')[0].setValue(true)
    await wrapper.get('[data-testid="batch-update-open"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="batch-field-status"]').setValue(true)
    await wrapper.get('[data-testid="batch-status-select"]').setValue('disabled')
    await wrapper.get('[data-testid="batch-field-notes"]').setValue(true)
    await wrapper.get('[data-testid="batch-notes-input"]').setValue('maintenance')
    await wrapper.get('#batch-update-redeem-form').trigger('submit')
    await flushPromises()

    expect(batchUpdateRedeemCodes).toHaveBeenCalledWith([1], {
      status: 'disabled',
      notes: 'maintenance'
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.redeem.batchUpdateSuccess')
  })

  // 生僻汉字按一个字符计数，超长输入在提交时提示并保留供用户修改。
  it.each([
    { name: '32 个常用汉字', code: '兑'.repeat(32), tooLong: false },
    { name: '32 个生僻汉字', code: '𠮷'.repeat(32), tooLong: false },
    { name: '32 个混合字符', code: '兑𠮷a🎁'.repeat(8), tooLong: false },
    { name: '首尾空白', code: ` \t${'𠮷'.repeat(32)}\n `, tooLong: false },
    { name: '33 个常用汉字', code: '兑'.repeat(33), tooLong: true },
    { name: '33 个生僻汉字', code: '𠮷'.repeat(33), tooLong: true },
    { name: '33 个英文字母', code: 'a'.repeat(33), tooLong: true }
  ])('按字符校验自定义兑换码：$name', async ({ code, tooLong }) => {
    const wrapper = mount(RedeemView, {
      attachTo: document.body,
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          Select: SelectStub,
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot /><slot name="footer" /></div>'
          },
          Icon: true,
          Teleport: true
        }
      }
    })

    try {
      await flushPromises()
      await wrapper.get('[data-testid="generate-open"]').trigger('click')
      const input = wrapper.get<HTMLInputElement>('[placeholder="admin.redeem.customCodePlaceholder"]')
      await input.setValue(code)
      await wrapper.get('[data-testid="generate-form"]').trigger('submit')
      await flushPromises()

      if (tooLong) {
        expect(generateRedeemCodes).not.toHaveBeenCalled()
        expect(showError).toHaveBeenCalledWith('admin.redeem.customCodeTooLong')
        expect(input.element.value).toBe(code)
        expect(wrapper.find('[data-testid="generate-form"]').exists()).toBe(true)
        return
      }
      expect(showError).not.toHaveBeenCalled()
      expect(generateRedeemCodes).toHaveBeenCalledTimes(1)
      expect(generateRedeemCodes.mock.calls[0][6]).toBe(code.trim())
      // jsdom 跳过 maxlength 的输入截断，需检查原生长度限制是否存在。
      expect(input.attributes('maxlength')).toBeUndefined()
    } finally {
      wrapper.unmount()
    }
  })

  it.each([false, true])('提交生成兑换码的付款条件：%s', async (requiresPayment) => {
    const wrapper = mount(RedeemView, {
      attachTo: document.body,
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          Select: SelectStub,
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot /><slot name="footer" /></div>'
          },
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="generate-open"]').trigger('click')
    await flushPromises()

    const maxUsesInput = wrapper.get('[data-testid="generate-max-uses"]')
    expect(maxUsesInput.exists()).toBe(true)

    expect(wrapper.get('#generateForm-requires-payment').attributes('aria-checked')).toBe('false')
    if (requiresPayment) {
      await wrapper.get('#generateForm-requires-payment').trigger('click')
    }
    await maxUsesInput.setValue('5')
    await wrapper.get('[data-testid="generate-form"]').trigger('submit')
    await flushPromises()

    expect(generateRedeemCodes).toHaveBeenCalledWith(
      1,
      'balance',
      10,
      undefined,
      5,
      null,
      undefined,
      undefined,
      requiresPayment
    )
  })
})
