import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CodexInviteResetModal from '../CodexInviteResetModal.vue'

const { getStatusMock, sendInviteMock, consumeMock, showSuccessMock, showErrorMock } = vi.hoisted(() => ({
  getStatusMock: vi.fn(),
  sendInviteMock: vi.fn(),
  consumeMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    providers: {
      getCodexInviteResetStatus: getStatusMock,
      sendCodexInviteResetInvite: sendInviteMock,
      consumeCodexInviteReset: consumeMock
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: showSuccessMock,
    showError: showErrorMock
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.providers.inviteResetEmailHint') return `max-${params?.max}`
        if (params?.time) return `${key}:${params.time}`
        return key
      }
    })
  }
})

const provider = {
  id: 42,
  name: 'OpenAI OAuth',
  platform: 'openai',
  type: 'oauth',
  status: 'active'
}

function mountModal() {
  return mount(CodexInviteResetModal, {
    props: {
      show: false,
      provider: provider as any
    },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>'
        },
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          template: '<div class="select-stub">{{ options?.[0]?.label }}</div>'
        },
        Icon: true
      }
    }
  })
}

describe('CodexInviteResetModal', () => {
  beforeEach(() => {
    getStatusMock.mockReset()
    sendInviteMock.mockReset()
    consumeMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
  })

  it('按收件人数检查名额，把管理员的明确同意传给当前活动，每次只提交一次', async () => {
    const eligibility = {
      referral_key: 'codex_referral_workspace', invite_available: true, requires_consent: true,
      available_invites: 1, available_count: 0, credits: []
    }
    getStatusMock.mockResolvedValueOnce(eligibility).mockResolvedValueOnce({ ...eligibility, available_invites: 0, invite_available: false })
    let finish!: (value: { sent_count: number }) => void
    sendInviteMock.mockReturnValue(new Promise((resolve) => { finish = resolve }))
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const send = wrapper.findAll('button').find((button) => button.text().includes('inviteResetSendInvite'))!
    const consume = wrapper.findAll('button').find((button) => button.text().includes('inviteResetUseReset'))!
    expect(consume.attributes('disabled')).toBeDefined()
    await wrapper.get('textarea').setValue('one@example.com')
    expect(send.attributes('disabled')).toBeDefined()
    await wrapper.get('input[type=checkbox]').setValue(true)
    await wrapper.get('textarea').setValue('one@example.com two@example.com')
    expect(send.attributes('disabled')).toBeDefined()
    await wrapper.get('textarea').setValue('one@example.com')
    expect(send.attributes('disabled')).toBeDefined()
    await wrapper.get('input[type=checkbox]').setValue(true)
    await Promise.all([send.trigger('click'), send.trigger('click')])
    expect(sendInviteMock).toHaveBeenCalledTimes(1)
    expect(sendInviteMock).toHaveBeenCalledWith(42, ['one@example.com'], 'codex_referral_workspace', true)
    finish({ sent_count: 1 })
    await flushPromises()
    expect(getStatusMock).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('admin.providers.inviteResetInviteSuccess')
    expect(send.attributes('disabled')).toBeDefined()
  })

  it('提交结果不明时保留邮箱并清除发送资格，手动刷新前不允许再发', async () => {
    getStatusMock.mockResolvedValue({
      referral_key: 'codex_referral_consumer', invite_available: true, requires_consent: false,
      available_invites: 1, available_count: 0, credits: []
    })
    sendInviteMock.mockRejectedValue({ message: '请先核对发送记录' })
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.get('textarea').setValue('one@example.com')
    const send = wrapper.findAll('button').find((button) => button.text().includes('inviteResetSendInvite'))!
    await send.trigger('click')
    await flushPromises()
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请先核对发送记录')
    expect(send.attributes('disabled')).toBeDefined()
    expect(wrapper.get('textarea').element.value).toBe('one@example.com')
    expect(sendInviteMock).toHaveBeenCalledTimes(1)
  })

  it('影子或 API Key 提供商不读取邀请资格', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true, provider: { ...provider, parent_provider_id: 1 } as any })
    await wrapper.setProps({ provider: { ...provider, type: 'apikey' } as any })
    await flushPromises()
    expect(getStatusMock).not.toHaveBeenCalled()
  })

  it('邀请入口不可用时只禁用发送邀请，保留已有重置次数操作', async () => {
    getStatusMock.mockResolvedValue({
      referral_key: 'codex_referral_consumer',
      invite_available: false,
      invite_unavailable_reason: 'CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE',
      invite_unavailable_message: '当前 Codex 推荐邀请入口暂不可用，但已有重置次数仍可使用',
      requires_consent: true,
      available_count: 1,
      credits: [
        {
          id: 'credit-1',
          status: 'available',
          title: 'Reset',
          expires_at: '2026-07-03T04:05:06Z'
        }
      ]
    })

    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('当前 Codex 推荐邀请入口暂不可用，但已有重置次数仍可使用')

    const buttons = wrapper.findAll('button')
    const consumeButton = buttons.find((button) => button.text().includes('admin.providers.inviteResetUseReset'))
    const sendButton = buttons.find((button) => button.text().includes('admin.providers.inviteResetSendInvite'))

    expect(consumeButton?.attributes('disabled')).toBeUndefined()
    expect(sendButton?.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('admin.providers.inviteResetCreditExpirations')
    expect(wrapper.text()).toContain('admin.providers.inviteResetCreditExpiresAtFull:')

    await sendButton!.trigger('click')
    expect(sendInviteMock).not.toHaveBeenCalled()
  })

  it('无奖励邀请显示无奖励而不是 Workspace credits', async () => {
    getStatusMock.mockResolvedValue({
      referral_key: 'codex_referral_consumer',
      invite_available: true,
      requires_consent: true,
      has_rewards: false,
      grant_type: 'none',
      available_count: 0,
      credits: []
    })

    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.providers.inviteResetGrantTypeNone')
    expect(wrapper.text()).not.toContain('admin.providers.inviteResetGrantTypeWorkspaceCredits')
  })

  it('按过期时间升序选择默认重置机会', async () => {
    getStatusMock.mockResolvedValue({
      referral_key: 'codex_referral_consumer',
      invite_available: true,
      requires_consent: true,
      available_count: 2,
      credits: [
        { id: 'credit-late', status: 'available', expires_at: '2026-07-05T04:05:06Z' },
        { id: 'credit-early', status: 'available', expires_at: '2026-07-03T04:05:06Z' }
      ]
    })
    consumeMock.mockResolvedValue({
      code: 'reset',
      redeem_request_id: 'redeem-1',
      windows_reset: 1
    })

    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const consumeButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('admin.providers.inviteResetUseReset'))
    await consumeButton!.trigger('click')
    await flushPromises()

    expect(consumeMock).toHaveBeenCalledWith(42, 'credit-early')
  })

  it('usage 有次数但没有明细时使用通用重置', async () => {
    getStatusMock.mockResolvedValue({
      referral_key: 'codex_referral_consumer',
      invite_available: false,
      invite_unavailable_message: '邀请暂不可用',
      requires_consent: true,
      available_count: 1,
      credits: []
    })
    consumeMock.mockResolvedValue({
      code: 'reset',
      redeem_request_id: 'redeem-2',
      windows_reset: 1
    })

    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.providers.inviteResetCreditDetailsUnavailable')
    const consumeButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('admin.providers.inviteResetUseReset'))
    expect(consumeButton?.attributes('disabled')).toBeUndefined()

    await consumeButton!.trigger('click')
    await flushPromises()

    expect(consumeMock).toHaveBeenCalledWith(42, undefined)
  })
})
