import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import type { Provider } from '@/types'
import { useAuthStore } from '@/stores/auth'
import { getAvailableModels } from '@/api/admin/providers'
import { listQuestionDefaults } from '@/api/admin/pelicanTests'
import { pelicanEfforts, type PelicanEffort } from '@/api/pelicanTests'
import {
  isIQTextModel, isSpecialOpenAIAuth, isStateProbeProvider, isStateProbeTextModel, runIQQuestion, runIQQuality, runIQStateProbe, runIQTrace, stateProbeDefaultModel, validStateProbeModelName,
  type IQQualityResult, type IQQuestionInput, type IQStateProbeResult, type IQTraceResult, type IQTraceSample,
} from '@/api/admin/providerIQ'
import { usePelicanResource } from '@/components/pelican/usePelicanResource'
import { defaultProviderTestModel } from '@/utils/providerDefaults'
import { iqHistoryKey, readIQHistory, saveIQHistory, type IQMode, type IQRecord, type IQSlot } from './providerIQHistory'

const modeNames: IQMode[] = ['question', 'probe', 'quality', 'modeltrace']
const questionKinds = ['candy', 'pelican', 'custom_text', 'custom_svg'] as const
type QuestionKind = typeof questionKinds[number]
const levels = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra']
const bytes = (value: string) => new TextEncoder().encode(value).length
const counts = Array.from({ length: 8 }, (_, i) => ({ value: i + 1, label: String(i + 1) }))

function requestIssue(error: unknown, probe = false): string {
  const failure = error as { status?: number; reason?: string } | null
  const status = failure?.status
  if (probe && status === 409) return 'providerIQ.probe.busy'
  if (status === 504 || failure?.reason === 'IQ_TEST_TIMEOUT') return 'providerIQ.timeout'
  if (failure?.reason === 'IQ_TEST_UNSUPPORTED') return 'providerIQ.unsupported'
  if (status === 409 || status === 429) return 'providerIQ.busy'
  if (status === 400 || status === 422) return 'providerIQ.rejected'
  if (status === 401 || status === 403 || status === 404 || status === 503) return 'providerIQ.unavailable'
  return 'providerIQ.failed'
}

export function useProviderIQTest(props: { show: boolean; provider: Provider | null }) {
  const { t, locale } = useI18n()
  const auth = useAuthStore()
  const mode = ref<IQMode>('question')
  const tab = ref<'results' | 'history'>('results')
  const model = ref('')
  const requestedModel = computed(() => model.value.trim() || (mode.value === 'probe' ? stateProbeDefaultModel : ''))
  const effort = ref('')
  const count = ref(1)
  const kind = ref<QuestionKind>('candy')
  const draftPrompt = ref<string | null>(null)
  const draftExpected = ref<string | null>(null)
  const slots = ref<IQSlot[]>([])
  const quality = shallowRef<IQQualityResult | null>(null)
  const probe = shallowRef<IQStateProbeResult | null>(null)
  const trace = shallowRef<IQTraceResult | null>(null)
  const traceSamples = shallowRef<IQTraceSample[]>([])
  const traceCount = ref(0)
  const issue = ref('')
  const cancelled = ref(false)
  const saveFailed = ref(false)
  const history = shallowRef<IQRecord[]>([])
  const completed = shallowRef<IQRecord | null>(null)
  const active = shallowRef<AbortController | null>(null)
  const busy = computed(() => !!active.value)
  const session = computed(() => props.show && props.provider && auth.user?.id ? iqHistoryKey(auth.user.id, props.provider.id) : null)
  const { data: catalog } = usePelicanResource(() => session.value, () => getAvailableModels(props.provider!.id))
  const { data: defaults } = usePelicanResource(() => session.value ? locale.value : null, (_language, signal) => listQuestionDefaults(signal))
  const models = computed(() => (catalog.value || []).map(item => ({ value: item.id, label: item.display_name && item.display_name !== item.id ? `${item.id} (${item.display_name})` : item.id })))
  watch(catalog, items => {
    if (!model.value && items?.length) model.value = defaultProviderTestModel(items) || items[0].id
  })

  const modes = computed(() => modeNames.map(value => ({ value, label: t('providerIQ.modes.' + value), testid: 'iq-mode-' + value })))
  const tabs = computed(() => (['results', 'history'] as const).map(value => ({ value, label: t('providerIQ.' + value), testid: 'iq-' + value })))
  const kinds = computed(() => questionKinds.map(value => ({ value, label: t('pelicanTests.kind.' + value) })))
  const efforts = computed(() => {
    const supported = props.provider && ['openai', 'anthropic'].includes(props.provider.platform) ? levels : []
    return [{ value: '', label: t('pelicanTests.effort.default') }, ...supported.map(value => ({ value, label: value === 'none' ? t('pelicanTests.effort.none') : value }))]
  })
  const custom = computed(() => kind.value === 'custom_svg' || kind.value === 'custom_text')
  const textQuestion = computed(() => kind.value === 'candy' || kind.value === 'custom_text')
  const preset = computed(() => defaults.value?.find(item => item.question_kind === kind.value))
  const prompt = computed({ get: () => draftPrompt.value ?? preset.value?.question_prompt ?? '', set: (value: string) => { draftPrompt.value = value } })
  const expected = computed({ get: () => draftExpected.value ?? preset.value?.expected_answer ?? '', set: (value: string) => { draftExpected.value = value } })
  const probeIssue = computed(() => {
    if (!props.provider || !isStateProbeProvider(props.provider)) return 'providerIQ.probe.unsupported'
    if (!validStateProbeModelName(requestedModel.value)) return 'providerIQ.probe.modelFormat'
    if (!isStateProbeTextModel(props.provider, requestedModel.value)) return 'providerIQ.probe.textOnly'
    return ''
  })
  const traceIssue = computed(() => {
    const provider = props.provider
    if (!provider || provider.parent_provider_id != null || !['openai', 'anthropic'].includes(provider.platform) || !['apikey', 'oauth', 'setup-token'].includes(provider.type)) return 'providerIQ.traceUnsupported'
    if (provider.platform === 'openai' && provider.type === 'setup-token') return 'providerIQ.traceUnsupported'
    if (provider.platform === 'openai' && provider.type === 'oauth' && isSpecialOpenAIAuth(provider)) return 'providerIQ.traceUnsupported'
    const protocols = provider.credentials?.upstream_protocols
    if (Array.isArray(protocols)) {
      const textProtocols = provider.platform === 'anthropic' ? ['anthropic_messages'] :
        provider.type === 'apikey' ? ['openai_responses', 'openai_chat_completions'] : ['openai_responses']
      if (!protocols.some(protocol => textProtocols.includes(protocol))) return 'providerIQ.traceProtocol'
    }
    return model.value.trim() && !isIQTextModel(provider, model.value) ? 'providerIQ.traceTextOnly' : ''
  })
  const validation = computed(() => {
    if (mode.value === 'probe') return ''
    if (bytes(model.value.trim()) > 512 || /\p{Cc}/u.test(model.value.trim())) return 'providerIQ.invalidModel'
    if (mode.value !== 'question') return ''
    if (bytes(prompt.value) > 8192) return 'providerIQ.invalidPrompt'
    if (textQuestion.value && bytes(expected.value) > 1024) return 'providerIQ.invalidAnswer'
    return ''
  })
  const canRun = computed(() => !!session.value && !!requestedModel.value && !busy.value && !validation.value &&
    (mode.value !== 'probe' || !probeIssue.value) &&
    (mode.value !== 'modeltrace' || !traceIssue.value) && (mode.value !== 'question' ||
      ((!custom.value || !!prompt.value.trim()) && Number.isInteger(Number(count.value)) && Number(count.value) >= 1 && Number(count.value) <= 8)))
  const scheduleTo = computed(() => ({
    path: '/admin/pelican-tests',
    query: {
      provider_id: String(props.provider?.id || ''), question_kind: mode.value === 'probe' ? 'state_probe' : kind.value,
      ...(bytes(requestedModel.value) <= 128 ? { model_id: requestedModel.value } : {}),
      ...(mode.value !== 'probe' && pelicanEfforts.includes(effort.value as PelicanEffort) ? { reasoning_effort: effort.value } : {}),
    },
  }))
  const probeScheduleModelTooLong = computed(() => mode.value === 'probe' && !probeIssue.value && bytes(requestedModel.value) > 128)
  const canSchedule = computed(() => !!props.provider && !busy.value &&
    (mode.value !== 'probe' || !probeIssue.value && !probeScheduleModelTooLong.value))

  function clearResults() {
    completed.value = null
    slots.value = []
    quality.value = null
    probe.value = null
    trace.value = null
    traceSamples.value = []
    traceCount.value = 0
    issue.value = ''
    cancelled.value = false
    saveFailed.value = false
  }
  function cancel() {
    if (!active.value) return
    active.value.abort()
    active.value = null
    cancelled.value = true
    for (const slot of slots.value) if (slot.state === 'running') slot.state = 'cancelled'
  }
  function setMode(value: string | number | boolean | null) {
    if (busy.value || mode.value === value || !modeNames.includes(value as IQMode)) return
    clearResults()
    mode.value = value as IQMode
    tab.value = 'results'
  }
  function setKind(value: string | number | boolean | null) {
    if (busy.value || !questionKinds.includes(value as QuestionKind)) return
    kind.value = value as QuestionKind
    draftPrompt.value = null
    draftExpected.value = null
  }
  function showHistory(record: IQRecord) {
    if (busy.value) return
    clearResults()
    mode.value = record.mode
    tab.value = 'results'
    completed.value = record
    if (record.mode === 'question') slots.value = record.samples.map(result => ({ state: 'complete', result }))
    if (record.mode === 'quality') quality.value = record.result
    if (record.mode === 'probe') {
      model.value = record.result.requested_model
      probe.value = record.result
    }
    if (record.mode === 'modeltrace') {
      trace.value = record.result
      traceSamples.value = record.result.samples
    }
  }
  function save(key: string, entry: IQRecord) {
    completed.value = entry
    const result = saveIQHistory(key, entry)
    history.value = result.items
    saveFailed.value = !result.stored
  }
  function downloadRound() {
    if (!completed.value || !props.provider) return
    const data = { provider_id: props.provider.id, ...completed.value }
    saveAs(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json;charset=utf-8' }), `iq-${props.provider.id}-${Date.parse(completed.value.at)}.json`)
  }

  async function run() {
    const provider = props.provider
    const key = session.value
    if (!canRun.value || !provider || !key) return
    clearResults()
    tab.value = 'results'
    const controller = new AbortController()
    active.value = controller
    const current = () => active.value === controller && !controller.signal.aborted && session.value === key
    const modelInput = { model_id: requestedModel.value, reasoning_effort: effort.value }
    const roundId = globalThis.crypto.randomUUID?.() ?? Array.from(crypto.getRandomValues(new Uint32Array(4)), n => n.toString(16)).join('-')
    const record = { id: roundId, at: new Date().toISOString(), model: modelInput.model_id, effort: modelInput.reasoning_effort }
    try {
      if (mode.value === 'probe') {
        const result = await runIQStateProbe(provider.id, { model_id: modelInput.model_id }, controller.signal)
        if (!current()) return
        probe.value = result
        save(key, { ...record, mode: 'probe', result, at: result.finished_at, effort: '' })
      } else if (mode.value === 'quality') {
        const result = await runIQQuality(provider.id, modelInput, controller.signal)
        if (!current()) return
        quality.value = result
        save(key, { ...record, mode: 'quality', result })
      } else if (mode.value === 'modeltrace') {
        const result = await runIQTrace(provider.id, modelInput, controller.signal, event => {
          if (!current()) return
          if (event.type === 'modeltrace_start') traceCount.value = event.data.challenges.length
          if (event.type === 'modeltrace_sample') traceSamples.value = [...traceSamples.value, event.data]
        })
        if (!current()) return
        trace.value = result
        traceSamples.value = result.samples
        save(key, { ...record, mode: 'modeltrace', result })
      } else {
        // 同轮各请求共享题面和编号，切换页面或取消时由同一信号撤销。
        const input: IQQuestionInput = { ...modelInput, question_kind: kind.value, prompt: prompt.value.trim(), round_id: roundId }
        if (textQuestion.value) input.expected_answer = expected.value.trim()
        slots.value = Array.from({ length: Number(count.value) }, () => ({ state: 'running' }))
        await Promise.all(slots.value.map(async slot => {
          try {
            const result = await runIQQuestion(provider.id, input, controller.signal)
            if (current()) Object.assign(slot, { state: 'complete', result })
          } catch (error) {
            if (current()) Object.assign(slot, { state: 'failed', message: requestIssue(error) })
          }
        }))
        if (!current()) return
        const samples = slots.value.flatMap(slot => slot.result ? [slot.result] : [])
        if (samples.length) save(key, { ...record, mode: 'question', samples })
      }
    } catch (error) {
      if (current()) issue.value = requestIssue(error, mode.value === 'probe')
    } finally {
      if (active.value === controller) active.value = null
    }
  }

  watch(session, key => {
    cancel()
    clearResults()
    mode.value = 'question'
    tab.value = 'results'
    model.value = ''
    effort.value = ''
    count.value = 1
    setKind('candy')
    history.value = key ? readIQHistory(key) : []
  }, { immediate: true, flush: 'sync' })
  onBeforeUnmount(cancel)
  return { mode, modes, setMode, tab, tabs, model, models, effort, efforts, count, counts, kind, kinds, setKind, custom, textQuestion,
    prompt, expected, validation, scheduleTo, canSchedule, probeIssue, probeScheduleModelTooLong, traceIssue, slots, history, showHistory, quality, probe, trace, traceSamples,
    traceCount, busy, cancelled, issue, saveFailed, canRun, run, cancel, completed, downloadRound }
}
