import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const clipboardWriteTextMock = vi.fn()

const {
  listProfilesMock,
  createProfileMock,
  updateProfileMock,
  deleteProfileMock,
  collectorStatusMock,
  authenticatedIngressStatusMock,
  startCollectorMock,
  stopCollectorMock,
  createCollectorSessionMock,
  listCollectorCapturesMock,
  promoteCollectorCaptureMock,
  showSuccessMock,
  showErrorMock
} = vi.hoisted(() => ({
  listProfilesMock: vi.fn(),
  createProfileMock: vi.fn(),
  updateProfileMock: vi.fn(),
  deleteProfileMock: vi.fn(),
  collectorStatusMock: vi.fn(),
  authenticatedIngressStatusMock: vi.fn(),
  startCollectorMock: vi.fn(),
  stopCollectorMock: vi.fn(),
  createCollectorSessionMock: vi.fn(),
  listCollectorCapturesMock: vi.fn(),
  promoteCollectorCaptureMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: showSuccessMock,
    showInfo: vi.fn()
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    tlsFingerprintProfiles: {
      list: listProfilesMock,
      create: createProfileMock,
      update: updateProfileMock,
      delete: deleteProfileMock,
      collectorStatus: collectorStatusMock,
      authenticatedIngressStatus: authenticatedIngressStatusMock,
      startCollector: startCollectorMock,
      stopCollector: stopCollectorMock,
      createCollectorSession: createCollectorSessionMock,
      listCollectorCaptures: listCollectorCapturesMock,
      promoteCollectorCapture: promoteCollectorCaptureMock
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (params && typeof params.count !== 'undefined') {
          return `${key}:${params.count}`
        }
        return key
      }
    })
  }
})

import TLSFingerprintProfilesModal from '../TLSFingerprintProfilesModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const captureRecord = {
  id: 'cap-1',
  captured_at: '2026-05-28T10:00:00Z',
  client_kind: 'codex',
  source_kind: 'controlled_connect',
  capture_mode: 'controlled_connect',
  target_host: 'chatgpt.com',
  promotion_eligible: true,
  matching_sample_count: 3,
  blocked_reasons: [],
  request_path: '/capture/token/v1/responses',
  method: 'POST',
  user_agent: 'codex-cli/0.1.0',
  ja3_raw: '771,4865-4866,0-10-16,29,0',
  ja3_hash: '1234567890abcdef1234567890abcdef',
  negotiated_alpn: 'http/1.1',
  http_proto: 'HTTP/1.1',
  yaml: 'captured_profile:\n  name: "Codex CLI 2026"\n  enable_grease: true\n',
  headers_summary: {},
  stainless_summary: {},
  profile: {
    id: 0,
    name: 'Codex CLI 2026',
    description: '由内置 TLS 指纹收集器采集',
    enable_grease: true,
    cipher_suites: [4865, 4866],
    curves: [29, 23],
    point_formats: [0],
    signature_algorithms: [1027, 2052],
    signature_algorithms_cert: [],
    alpn_protocols: ['h2', 'http/1.1'],
    supported_versions: [772, 771],
    key_share_groups: [29],
    psk_modes: [1],
    extensions: [0, 10, 16, 43, 51],
    verification: {
      source_kind: 'controlled_connect',
      validation_status: 'draft',
      validator_version: 0
    },
    created_at: '',
    updated_at: ''
  }
}

function mountModal() {
  return mount(TLSFingerprintProfilesModal, {
    props: {
      show: true
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Icon: true
      }
    }
  })
}

describe('TLSFingerprintProfilesModal', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listProfilesMock.mockReset()
    createProfileMock.mockReset()
    updateProfileMock.mockReset()
    deleteProfileMock.mockReset()
    collectorStatusMock.mockReset()
    authenticatedIngressStatusMock.mockReset()
    startCollectorMock.mockReset()
    stopCollectorMock.mockReset()
    createCollectorSessionMock.mockReset()
    listCollectorCapturesMock.mockReset()
    promoteCollectorCaptureMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    clipboardWriteTextMock.mockReset()

    Object.defineProperty(navigator, 'clipboard', {
      value: {
        writeText: clipboardWriteTextMock
      },
      configurable: true
    })

    listProfilesMock.mockResolvedValue([])
    createProfileMock.mockResolvedValue({})
    promoteCollectorCaptureMock.mockResolvedValue({})
    collectorStatusMock.mockResolvedValue({
      running: false,
      listen_address: '127.0.0.1:8443',
      public_base_url: 'https://collector.example:8443',
      using_generated_cert: false,
      session_ttl_seconds: 1800,
      max_records_per_session: 20
    })
    authenticatedIngressStatusMock.mockResolvedValue({
      enabled: true,
      pending_client_hellos: 0,
      pending_authenticated_flows: 0,
      candidate_groups: 0,
      validations_in_flight: 0,
      matched_flows: 0,
      rejected_flows: 0,
      verified_profiles_created: 0,
      existing_profiles_matched: 0
    })
    startCollectorMock.mockResolvedValue({
      running: true,
      listen_address: '127.0.0.1:8443',
      public_base_url: 'https://collector.example:8443',
      using_generated_cert: true,
      ca_pem: '-----BEGIN CERTIFICATE-----\\nCA\\n-----END CERTIFICATE-----',
      session_ttl_seconds: 1800,
      max_records_per_session: 20
    })
    createCollectorSessionMock.mockResolvedValue({
      token: 'token-1',
      expires_at: '2026-05-28T10:30:00Z',
      capture_url: 'https://collector.example:8443/capture/token-1',
      proxy_url: 'https://capture:token-1@collector.example:8443',
      mode: 'controlled_connect',
      client_kind: 'codex',
      client_version: '0.144.1',
      operating_system: 'Ubuntu 22.04',
      architecture: 'x86_64',
      target_host: 'chatgpt.com',
      cold_start_confirmed: true,
      ca_pem: '-----BEGIN CERTIFICATE-----\\nCA\\n-----END CERTIFICATE-----'
    })
    listCollectorCapturesMock.mockResolvedValue([captureRecord])
  })

  it('可启动收集器、创建受控会话并验证创建模板', async () => {
    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.tlsFingerprintProfiles.collector.stopped')

    const startButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.tlsFingerprintProfiles.collector.start')
    )
    expect(startButton).toBeTruthy()
    await startButton!.trigger('click')
    await flushPromises()

    expect(startCollectorMock).toHaveBeenCalledTimes(1)
    expect(collectorStatusMock).toHaveBeenCalledTimes(2)

    collectorStatusMock.mockResolvedValue({
      running: true,
      listen_address: '127.0.0.1:8443',
      public_base_url: 'https://collector.example:8443',
      using_generated_cert: true,
      ca_pem: '-----BEGIN CERTIFICATE-----\\nCA\\n-----END CERTIFICATE-----',
      session_ttl_seconds: 1800,
      max_records_per_session: 20
    })
    await wrapper.findAll('button').find(button =>
      button.text().includes('common.refresh')
    )!.trigger('click')
    await flushPromises()

    const sessionButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.tlsFingerprintProfiles.collector.createSession')
    )
    expect(sessionButton).toBeTruthy()
    await wrapper.find('#tls-capture-client-version').setValue('0.144.1')
    await wrapper.find('#tls-capture-os').setValue('Ubuntu 22.04')
    await wrapper.find('#tls-capture-arch').setValue('x86_64')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await sessionButton!.trigger('click')
    await flushPromises()

    expect(createCollectorSessionMock).toHaveBeenCalledTimes(1)
    expect(createCollectorSessionMock).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'controlled_connect',
      client_kind: 'codex',
      target_host: 'chatgpt.com',
      cold_start_confirmed: true
    }))
    expect(listCollectorCapturesMock).toHaveBeenCalledWith('token-1')
    expect(wrapper.text()).toContain('codex-cli/0.1.0')
    expect(wrapper.text()).toContain('HTTPS_PROXY="https://capture:token-1@collector.example:8443"')
    expect(wrapper.text()).toContain('CODEX_CA_CERTIFICATE=/path/to/tokenrouter-tls-collector-ca.pem')

    const promoteButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.tlsFingerprintProfiles.collector.promoteCapture')
    )
    expect(promoteButton).toBeTruthy()
    await promoteButton!.trigger('click')
    await flushPromises()

    expect(promoteCollectorCaptureMock).toHaveBeenCalledWith('token-1', 'cap-1', { name: 'Codex CLI 2026' })
  })

  it('可在模板列表操作列复制模板 YAML', async () => {
    listProfilesMock.mockResolvedValue([
      {
        id: 12,
        name: 'Mac Codex',
        description: 'export me',
        enable_grease: true,
        cipher_suites: [4865, 4866],
        curves: [29, 23],
        point_formats: [0],
        signature_algorithms: [2052],
        signature_algorithms_cert: [],
        alpn_protocols: ['h2', 'http/1.1'],
        supported_versions: [772, 771],
        key_share_groups: [29],
        psk_modes: [1],
        extensions: [0, 43],
        verification: {
          source_kind: 'authenticated_ingress',
          validation_status: 'verified',
          validator_version: 1
        },
        created_at: '2026-06-01T00:00:00Z',
        updated_at: '2026-06-01T00:00:00Z'
      }
    ])

    const wrapper = mountModal()
    await flushPromises()

    const copyButton = wrapper.find('button[aria-label="admin.tlsFingerprintProfiles.copyYaml"]')
    expect(copyButton.exists()).toBe(true)
    expect(wrapper.get('[data-testid="tls-profile-source-12"]').text()).toBe(
      'admin.tlsFingerprintProfiles.verificationSource.authenticatedIngress'
    )

    await copyButton.trigger('click')
    await flushPromises()

    expect(clipboardWriteTextMock).toHaveBeenCalledWith([
      'tls_fingerprint_profile:',
      '  name: "Mac Codex"',
      '  description: "export me"',
      '  enable_grease: true',
      '  cipher_suites: [0x1301, 0x1302]',
      '  curves: [29, 23]',
      '  point_formats: [0]',
      '  signature_algorithms: [0x0804]',
      '  signature_algorithms_cert: []',
      '  alpn_protocols: ["h2", "http/1.1"]',
      '  supported_versions: [0x0304, 0x0303]',
      '  key_share_groups: [29]',
      '  psk_modes: [1]',
      '  extensions: [0x0000, 0x002b]'
    ].join('\n'))
    expect(showSuccessMock).toHaveBeenCalledWith('admin.tlsFingerprintProfiles.collector.copied')
  })

  it('展示已认证入站采集的聚合状态并支持手动刷新', async () => {
    authenticatedIngressStatusMock.mockResolvedValue({
      enabled: true,
      pending_client_hellos: 12,
      pending_authenticated_flows: 3,
      candidate_groups: 4,
      validations_in_flight: 2,
      matched_flows: 25,
      rejected_flows: 1,
      verified_profiles_created: 6,
      existing_profiles_matched: 19,
      last_verified_at: '2026-09-03T08:30:00Z',
      last_error: 'one candidate could not be replayed'
    })

    const wrapper = mountModal()
    await flushPromises()

    expect(authenticatedIngressStatusMock).toHaveBeenCalledTimes(1)
    const metrics = wrapper.get('[data-testid="authenticated-ingress-metrics"]')
    expect(metrics.text()).toContain('admin.tlsFingerprintProfiles.authenticatedIngress.pendingClientHellos')
    expect(metrics.text()).toContain('12')
    expect(metrics.text()).toContain('admin.tlsFingerprintProfiles.authenticatedIngress.verifiedProfilesCreated')
    expect(metrics.text()).toContain('6')
    expect(wrapper.get('[data-testid="authenticated-ingress-backend-error"]').text()).toContain(
      'one candidate could not be replayed'
    )

    await wrapper.get('[data-testid="authenticated-ingress-refresh"]').trigger('click')
    await flushPromises()
    expect(authenticatedIngressStatusMock).toHaveBeenCalledTimes(2)
  })

  it('已认证入站状态加载失败时显示可恢复错误，重试成功后显示空状态', async () => {
    const consoleErrorMock = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    authenticatedIngressStatusMock.mockRejectedValueOnce({
      response: { data: { detail: 'status temporarily unavailable' } }
    })

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.get('[data-testid="authenticated-ingress-error"]').text()).toContain('status temporarily unavailable')

    authenticatedIngressStatusMock.mockResolvedValue({
      enabled: true,
      pending_client_hellos: 0,
      pending_authenticated_flows: 0,
      candidate_groups: 0,
      validations_in_flight: 0,
      matched_flows: 0,
      rejected_flows: 0,
      verified_profiles_created: 0,
      existing_profiles_matched: 0
    })
    await wrapper.get('[data-testid="authenticated-ingress-error"] button').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="authenticated-ingress-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="authenticated-ingress-empty"]').text()).toContain(
      'admin.tlsFingerprintProfiles.authenticatedIngress.noActivity'
    )
    consoleErrorMock.mockRestore()
  })
})
