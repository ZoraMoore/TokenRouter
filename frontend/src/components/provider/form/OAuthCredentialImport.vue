<template>
  <div class="space-y-4">
    <p v-if="description" class="text-sm text-gray-600 dark:text-gray-300">{{ description }}</p>

    <div>
      <div class="mb-1.5 flex items-center gap-2">
        <label :for="uid" class="input-label mb-0">{{ label }}</label>
        <span
          v-if="count > 1"
          class="rounded-full bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-500/10 dark:text-primary-300"
        >
          {{ t('admin.providers.oauth.keysCount', { count }) }}
        </span>
        <slot name="label-extra" />
      </div>
      <input
        v-if="password"
        :id="uid"
        v-model="value"
        type="password"
        class="input font-mono text-sm"
        :placeholder="placeholder"
        autocomplete="off"
        spellcheck="false"
      />
      <textarea
        v-else
        :id="uid"
        v-model="value"
        :rows="rows"
        class="input resize-y font-mono text-sm"
        :placeholder="placeholder"
        spellcheck="false"
      ></textarea>
      <p v-if="count > 1 && countHint" class="input-hint">{{ countHint }}</p>
      <p v-if="hint" class="input-hint">{{ hint }}</p>
    </div>

    <slot />

    <SettingsNotice v-if="error" tone="error">
      <p class="whitespace-pre-line">{{ error }}</p>
    </SettingsNotice>

    <div class="flex justify-end">
      <button
        type="button"
        class="btn btn-primary"
        :disabled="loading || !value.trim()"
        @click="emit('submit')"
      >
        <Icon v-if="loading" name="loader" size="sm" :animate-on-hover="false" class="animate-spin" />
        <Icon v-else name="sparkles" size="sm" />
        {{ loading ? loadingLabel : submitLabel }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useId } from 'vue'
import { useI18n } from 'vue-i18n'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import Icon from '@/components/icons/Icon.vue'

// Refresh Token、SSO、Codex 会话、PAT、Cookie 等“粘贴凭据后创建”的表单共用同一结构。
withDefaults(defineProps<{
  label: string
  submitLabel: string
  loadingLabel: string
  description?: string
  placeholder?: string
  hint?: string
  /** 解析出的条目数，大于 1 时显示数量和批量提示。 */
  count?: number
  countHint?: string
  rows?: number
  /** 单个令牌使用密码输入框，其余使用多行文本。 */
  password?: boolean
  loading?: boolean
  error?: string
}>(), {
  count: 0,
  rows: 3,
})
const value = defineModel<string>({ required: true })
const emit = defineEmits<{ submit: [] }>()

const { t } = useI18n()
const uid = useId()
</script>
