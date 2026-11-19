<template>
  <BaseDialog :show="show" :title="t(plan ? 'pelicanTests.editPlan' : 'pelicanTests.createPlan')" @close="close">
    <ContentSkeleton v-if="groupsLoading && !groups || targetLoading" variant="form" :rows="5" />
    <form v-else id="pelican-plan-form" novalidate class="space-y-4" @submit.prevent="save">
      <SettingsNotice v-if="groupsFailed" tone="error">
        {{ t('pelicanTests.groupsLoadFailed') }}
        <button type="button" class="ml-2 underline" @click="refreshGroups">{{ t('common.retry') }}</button>
      </SettingsNotice>
      <SettingsNotice v-else-if="targetFailed" tone="error">
        {{ t('pelicanTests.providersLoadFailed') }}
        <button type="button" class="ml-2 underline" @click="refreshTarget">{{ t('common.retry') }}</button>
      </SettingsNotice>
      <SettingsNotice v-else-if="!plan && !groupOptions.length">{{ t('common.noGroupsAvailable') }}</SettingsNotice>
      <SettingRow id="pelican-group-row" label-for="pelican-group" :label="t('pelicanTests.group')" field>
        <Select
          id="pelican-group" :model-value="form.group_id" :options="groupOptions" searchable
          :disabled="!!plan || saving" :aria-label="t('pelicanTests.group')" @update:model-value="selectGroup"
        />
      </SettingRow>

      <SettingRow id="pelican-provider-row" label-for="pelican-provider" :label="t('pelicanTests.provider')" field>
        <Select
          id="pelican-provider" v-model="form.provider_id" :options="providerOptions" searchable
          :aria-label="t('pelicanTests.provider')" :disabled="!!target || !form.group_id || providersLoading || saving"
        />
      </SettingRow>
      <SettingsNotice v-if="providersFailed" tone="warning">
        {{ t('pelicanTests.providersLoadFailed') }}
        <button type="button" class="ml-2 underline" @click="refreshProviders">{{ t('common.retry') }}</button>
      </SettingsNotice>
      <SettingsNotice v-else-if="target && form.group_id && providers && targetUnavailable" tone="warning">
        {{ t('pelicanTests.targetUnavailable') }}
      </SettingsNotice>
      <SettingsNotice v-else-if="isProbe && form.group_id && providers && !eligibleProviders.length" tone="warning">
        {{ t('pelicanTests.probe.noProvider') }}
      </SettingsNotice>

      <div>
        <label for="pelican-model" class="input-label">{{ t('pelicanTests.model') }}</label>
        <input
          id="pelican-model" v-model="form.model_id" required maxlength="128" spellcheck="false"
          class="input font-mono" list="pelican-model-options" :disabled="saving"
          :placeholder="t('pelicanTests.modelPlaceholder')"
        />
        <datalist id="pelican-model-options">
          <option v-for="model in candidates" :key="model" :value="model" />
        </datalist>
      </div>
      <SettingRow v-if="!isProbe" id="pelican-effort-row" label-for="pelican-effort" :label="t('pelicanTests.reasoningEffort')" field>
        <Select
          id="pelican-effort" v-model="form.reasoning_effort" :options="effortOptions" :disabled="saving"
          :aria-label="t('pelicanTests.reasoningEffort')"
        />
      </SettingRow>
      <SettingRow
        v-if="!plan && !isProbe && templates.length" id="pelican-template-row" label-for="pelican-question-template"
        :label="t('pelicanTests.copyQuestion')" field
      >
        <Select
          id="pelican-question-template" :model-value="templateId" :options="templates" searchable
          :placeholder="t('pelicanTests.chooseTemplate')" :disabled="saving" :aria-label="t('pelicanTests.copyQuestion')"
          @update:model-value="copyQuestion"
        />
      </SettingRow>
      <div>
        <span class="input-label">{{ t('pelicanTests.questionKind') }}</span>
        <SettingsSegmented
          :model-value="form.question_kind" :options="questionOptions" :disabled="saving"
          :ariaLabel="t('pelicanTests.questionKind')" block @update:model-value="selectKind"
        />
      </div>
      <SettingsNotice v-if="rotatingQuestion">{{ t('pelicanTests.rotatingQuestionHint') }}</SettingsNotice>
      <template v-if="isProbe">
        <SettingsNotice>{{ t('pelicanTests.probe.hint') }}</SettingsNotice>
        <div>
          <label for="pelican-probe-name" class="input-label">{{ t('pelicanTests.probe.name') }}</label>
          <input id="pelican-probe-name" v-model="questionName" maxlength="96" class="input" :disabled="saving" />
        </div>
      </template>
      <ContentSkeleton v-else-if="missingDefaults && loadingDefaults" variant="form" :rows="2" />
      <SettingsNotice v-else-if="missingDefaults" tone="error">
        {{ t('pelicanTests.defaultsLoadFailed') }}
        <button type="button" data-testid="retry-defaults" class="ml-2 underline" @click="refreshDefaults">{{ t('common.retry') }}</button>
      </SettingsNotice>
      <template v-else>
        <div>
          <label for="pelican-question-name" class="input-label">{{ t('pelicanTests.questionName') }}</label>
          <input id="pelican-question-name" v-model="questionName" maxlength="96" required class="input" :disabled="saving" />
        </div>
        <div>
          <label for="pelican-question-prompt" class="input-label">{{ t('pelicanTests.questionPrompt') }}</label>
          <textarea id="pelican-question-prompt" v-model="questionPrompt" rows="4" maxlength="8192" required class="input" :disabled="saving" />
        </div>
        <div v-if="!isPelicanArtwork(form.question_kind)">
          <label for="pelican-expected-answer" class="input-label">{{ t('pelicanTests.expectedAnswer') }}</label>
          <textarea id="pelican-expected-answer" v-model="expectedAnswer" rows="2" maxlength="1024" required class="input" :disabled="saving" />
        </div>
      </template>
      <div>
        <label for="pelican-frequency" class="input-label">{{ t('pelicanTests.testFrequency') }}</label>
        <Select
          id="pelican-frequency" :model-value="frequency" :options="frequencyOptions" searchable
          :aria-label="t('pelicanTests.testFrequency')" :disabled="saving" @update:model-value="selectFrequency"
        />
        <p class="input-hint">{{ t('pelicanTests.frequencyHint') }}</p>
      </div>
      <div v-if="frequency === 'custom'">
        <label for="pelican-cron" class="input-label">{{ t('pelicanTests.cron') }}</label>
        <input
          id="pelican-cron" v-model="form.cron_expression" class="input font-mono" required maxlength="128"
          spellcheck="false" :disabled="saving" aria-describedby="pelican-cron-hint"
        />
        <p id="pelican-cron-hint" class="input-hint">{{ t('pelicanTests.cronHint') }}</p>
      </div>
      <SettingToggleRow
        id="pelican-enabled" v-model="form.enabled" :disabled="saving"
        :label="t('pelicanTests.schedule')" :hint="t('pelicanTests.scheduleHint')"
      />
      <SettingsNotice v-if="error" tone="error">{{ t(error) }}</SettingsNotice>
    </form>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
        <button type="submit" form="pelican-plan-form" class="btn btn-primary" :disabled="saving || !form.group_id || groupsLoading || missingDefaults || targetUnavailable || probeNeedsProvider">
          {{ t(saving ? 'common.saving' : 'common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getOptions, getModelsListCandidates } from '@/api/admin/groups'
import { getById } from '@/api/admin/providers'
import { createPlan, listProviders, listQuestionDefaults, updatePlan, type PelicanPlan, type PelicanPlanInput, type PelicanPlanTarget, type PelicanQuestionDefault } from '@/api/admin/pelicanTests'
import { isCustomQuestion, isPelicanArtwork, pelicanEfforts, pelicanPlanKinds, type PelicanPlanKind } from '@/api/pelicanTests'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import SettingRow from '@/components/common/settings/SettingRow.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import SettingsSegmented from '@/components/common/settings/SettingsSegmented.vue'
import { frequencyForCron, pelicanFrequencies, type PelicanFrequency } from './pelicanFrequency'
import { usePelicanResource } from './usePelicanResource'

const props = defineProps<{ show: boolean; plan: PelicanPlan | null; plans: PelicanPlan[]; target?: PelicanPlanTarget | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t, locale } = useI18n()
type Draft = Omit<PelicanPlanInput, 'group_id'> & { group_id: number | null }
const questionFields = ['question_name', 'question_prompt', 'expected_answer'] as const
type QuestionField = typeof questionFields[number]
type QuestionDraft = Pick<PelicanPlanInput, QuestionField>

function storedQuestion(plan?: QuestionDraft | null): QuestionDraft {
  return {
    question_name: plan?.question_name || undefined,
    question_prompt: plan?.question_prompt || undefined,
    expected_answer: plan?.expected_answer || undefined,
  }
}

function draft(plan: PelicanPlan | null): Draft {
  return {
    group_id: plan?.group_id ?? null,
    provider_id: plan?.provider_id ?? null,
    model_id: plan?.model_id ?? '',
    question_kind: plan?.question_kind ?? 'pelican',
    ...storedQuestion(plan),
    reasoning_effort: plan?.reasoning_effort ?? '',
    cron_expression: plan?.cron_expression ?? pelicanFrequencies.daily,
    enabled: plan?.enabled ?? true,
  }
}

const form = ref(draft(null))
const templateId = ref<number | null>(null)
const frequency = ref<PelicanFrequency>('daily')
const customCron = ref('')
const draftsByKind = new Map<PelicanPlanKind, QuestionDraft>()
const isProbe = computed(() => form.value.question_kind === 'state_probe')
const error = ref('')
const saving = ref(false)
watch([() => props.show, () => props.target], ([open]) => {
  if (open) {
    templateId.value = null
    form.value = { ...draft(props.plan), ...props.target }
    frequency.value = frequencyForCron(form.value.cron_expression)
    customCron.value = frequency.value === 'custom' ? form.value.cron_expression : ''
    draftsByKind.clear()
    error.value = ''
  }
}, { immediate: true })

// 缓存随管理页面释放。语言变更时重新取题，正在输入的字段仍由草稿保存。
const defaultsByLocale = new Map<string, PelicanQuestionDefault[]>()
const { data: defaults, loading: loadingDefaults, refresh: refreshDefaults } =
  usePelicanResource(() => props.show && !isProbe.value ? locale.value : null, async (language, signal) => {
    const cached = defaultsByLocale.get(language)
    if (cached) return cached
    const items = await listQuestionDefaults(signal)
    if (!signal.aborted) defaultsByLocale.set(language, items)
    return items
  })
const defaultQuestion = computed(() => defaults.value?.find(item => item.question_kind === form.value.question_kind))
const missingDefaults = computed(() => !isProbe.value && !isCustomQuestion(form.value.question_kind) && !defaultQuestion.value &&
  (!form.value.question_name || !form.value.question_prompt || !isPelicanArtwork(form.value.question_kind) && !form.value.expected_answer))
const rotatingQuestion = computed(() => props.plan?.question_kind === 'pelican' && !props.plan.question_prompt && form.value.question_kind === 'pelican')

// 未覆盖的字段直接显示接口默认值。输入后保存在草稿里，保存时按原计划的覆盖情况写回。
function questionField(key: QuestionField) {
  return computed({
    get: () => form.value[key] ?? defaultQuestion.value?.[key] ?? '',
    set: (value: string) => { form.value[key] = value },
  })
}
const questionName = questionField('question_name')
const questionPrompt = questionField('question_prompt')
const expectedAnswer = questionField('expected_answer')

const frequencyOptions = computed(() => [...Object.keys(pelicanFrequencies), 'custom']
  .map(value => ({ value, label: t(`pelicanTests.frequency.${value}`) })))

function selectFrequency(value: unknown) {
  if (typeof value !== 'string' || value === frequency.value) return
  if (!frequencyOptions.value.some(option => option.value === value)) return
  if (frequency.value === 'custom') customCron.value = form.value.cron_expression
  frequency.value = value as PelicanFrequency
  form.value.cron_expression = frequency.value === 'custom'
    ? customCron.value || form.value.cron_expression
    : pelicanFrequencies[frequency.value]
}

const { data: groups, loading: groupsLoading, failed: groupsFailed, refresh: refreshGroups } =
  usePelicanResource(() => props.show && !props.plan ? true : null, () => getOptions())
const { data: targetProvider, loading: targetLoading, failed: targetFailed, refresh: refreshTarget } =
  usePelicanResource(() => props.show ? props.target?.provider_id ?? null : null, getById)
const groupOptions = computed(() => {
  if (props.plan) return [{ value: props.plan.group_id, label: props.plan.group_name }]
  const membership = targetProvider.value?.group_ids ?? targetProvider.value?.groups?.map(group => group.id) ?? []
  return (groups.value || [])
    .filter(group => !props.target || membership.includes(group.id))
    .map(group => ({ value: group.id, label: group.name }))
})
watch(groupOptions, options => {
  if (props.target && !form.value.group_id && options.length === 1) selectGroup(options[0].value)
})
const groupForOptions = () => props.show ? form.value.group_id : null
const { data: providers, loading: providersLoading, failed: providersFailed, refresh: refreshProviders } =
  usePelicanResource(groupForOptions, listProviders)
const { data: candidates } = usePelicanResource(groupForOptions, getModelsListCandidates)
const eligibleProviders = computed(() => (providers.value || []).filter(provider => !isProbe.value || provider.supports_state_probe))
const probeNeedsProvider = computed(() => isProbe.value && !eligibleProviders.value.some(provider => provider.id === form.value.provider_id))
const targetUnavailable = computed(() => !!props.target && (
  !targetProvider.value || !eligibleProviders.value.some(provider => provider.id === props.target?.provider_id)
))
const providerOptions = computed(() => {
  if (props.target) return [{ value: props.target.provider_id, label: targetProvider.value?.name || '#' + props.target.provider_id }]
  const choices: SelectOption[] = eligibleProviders.value.map(provider => ({ value: provider.id, label: provider.name }))
  const plan = props.plan
  // 提供商已解绑时，编辑器仍显示计划当前指向的提供商。
  if (plan?.provider_id && !choices.some(item => item.value === plan.provider_id)) {
    const name = plan.provider_name || '#' + plan.provider_id
    const detached = providers.value !== null
    choices.push({
      value: plan.provider_id, label: detached ? t('pelicanTests.detachedProvider', { name }) : name, disabled: detached,
    })
  }
  return isProbe.value ? choices : [{ value: null, label: t('pelicanTests.autoProvider') }, ...choices]
})
const questionOptions = computed(() => pelicanPlanKinds.map(value => ({ value, label: t(`pelicanTests.kind.${value}`) })))
const effortOptions = computed(() => [
  { value: '', label: t('pelicanTests.effort.default') },
  ...pelicanEfforts.map(value => ({ value, label: t(`pelicanTests.effort.${value}`) })),
])

// 多个分组可以复用同一道题。选项合并相同的题面，复用时仅复制题目字段。
const templates = computed(() => {
  const seen = new Set<string>()
  return props.plans.filter(plan => {
    if (plan.question_kind === 'state_probe' || !questionFields.some(key => plan[key])) return false
    const key = JSON.stringify([plan.question_kind, plan.question_name, plan.question_prompt, plan.expected_answer])
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).map(plan => ({
    value: plan.id,
    label: `${plan.question_name || t(`pelicanTests.kind.${plan.question_kind}`)} · ${plan.group_name} · ${plan.model_id}`,
  }))
})

function copyQuestion(id: unknown) {
  const source = props.plans.find(plan => plan.id === id)
  if (props.plan || !source) return
  selectKind(source.question_kind)
  templateId.value = source.id
  Object.assign(form.value, storedQuestion(source))
}

function selectGroup(value: unknown) {
  if (props.plan || saving.value || typeof value !== 'number' || value === form.value.group_id) return
  form.value.group_id = value
  form.value.provider_id = props.target?.provider_id ?? null
}

function selectKind(value: PelicanPlanKind | null) {
  if (!value || value === form.value.question_kind) return
  const previous = form.value.question_kind
  const fields = { question_name: form.value.question_name, question_prompt: form.value.question_prompt, expected_answer: form.value.expected_answer }
  draftsByKind.set(previous, fields)
  templateId.value = null
  form.value.question_kind = value
  if (value === 'state_probe' && !props.target && probeNeedsProvider.value) form.value.provider_id = null
  // 文本题与绘图题互换时，自定义题的名称和题面可继续使用。
  const next = draftsByKind.get(value) ?? (isCustomQuestion(previous) && isCustomQuestion(value) ? fields : storedQuestion())
  Object.assign(form.value, next)
  if (isPelicanArtwork(value)) form.value.expected_answer = undefined
}

function close() {
  if (!saving.value) emit('close')
}

const bytes = new TextEncoder()

function validQuestionText(input: string | undefined, limit: number, multiline = false): boolean {
  const text = input?.trim() || ''
  const singleLine = multiline ? text.replace(/[\r\n\t]/g, '') : text
  return Boolean(text) && bytes.encode(text).length <= limit && !/\p{Cc}/u.test(singleLine)
}

async function save() {
  if (saving.value || missingDefaults.value || targetUnavailable.value || probeNeedsProvider.value) return
  const group = form.value.group_id
  if (!groupOptions.value.some(option => option.value === group) || group === null) {
    error.value = 'pelicanTests.chooseGroup'
    return
  }
  const providerId = form.value.provider_id
  if (providerId !== null && providers.value && !providers.value.some(provider => provider.id === providerId)) {
    error.value = 'pelicanTests.chooseProvider'
    return
  }
  const model = form.value.model_id.trim()
  const cron = form.value.cron_expression.trim()
  if (!model || /[\s*\p{Cc}]/u.test(model) || bytes.encode(model).length > 128) {
    error.value = 'pelicanTests.invalidModel'
    return
  }
  if (cron.split(/\s+/).length !== 5 || bytes.encode(cron).length > 128) {
    error.value = 'pelicanTests.invalidCron'
    return
  }
  if ((!isProbe.value || questionName.value.trim()) && !validQuestionText(questionName.value, 96)) error.value = 'pelicanTests.invalidQuestionName'
  else if (!isProbe.value && !validQuestionText(questionPrompt.value, 8192, true)) error.value = 'pelicanTests.invalidQuestionPrompt'
  else if (!isProbe.value && !isPelicanArtwork(form.value.question_kind) && !validQuestionText(expectedAnswer.value, 1024, true)) {
    error.value = 'pelicanTests.invalidExpectedAnswer'
  } else {
    error.value = ''
  }
  if (error.value) return

  const input: PelicanPlanInput = {
    ...form.value, group_id: group, model_id: model, cron_expression: cron,
    question_name: questionName.value.trim(), question_prompt: questionPrompt.value.trim(), expected_answer: expectedAnswer.value.trim(),
  }
  // 原计划未覆盖的字段与默认值相同时继续留空，保留鹈鹕题库轮换和糖果题的默认判分。
  const previous = props.plan
  if (previous?.question_kind === input.question_kind && !isCustomQuestion(input.question_kind)) {
    for (const key of questionFields) {
      if (!previous[key] && input[key] === defaultQuestion.value?.[key]) delete input[key]
    }
  }
  saving.value = true
  error.value = ''
  try {
    if (props.plan) await updatePlan(props.plan.id, input)
    else await createPlan(input)
    emit('saved')
  } catch {
    error.value = 'pelicanTests.saveFailed'
  } finally {
    saving.value = false
  }
}
</script>
