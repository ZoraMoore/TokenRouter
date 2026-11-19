<template>
  <component
    :is="collapsible ? 'details' : 'section'" v-if="sample.prompt || sample.criteria?.length"
    class="rounded-surface border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
    :class="collapsible ? 'max-h-[24dvh] shrink-0 overflow-y-auto px-3 py-2' : 'p-4'"
  >
    <component :is="collapsible ? 'summary' : 'h3'" class="text-sm font-semibold text-gray-900 dark:text-dark-50">
      {{ t(isProbe ? 'pelicanTests.probe.method' : 'pelicanTests.prompt') }}
    </component>
    <p class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-700 dark:text-dark-200">{{ sample.prompt }}</p>
    <template v-if="sample.criteria?.length">
      <h4 class="mb-1 mt-4 text-sm font-semibold text-gray-900 dark:text-dark-50">{{ t(isProbe ? 'pelicanTests.probe.interpretation' : 'pelicanTests.criteria') }}</h4>
      <ul class="list-disc space-y-1 pl-5 text-sm text-gray-700 dark:text-dark-200">
        <li v-for="(criterion, index) in sample.criteria" :key="index">{{ criterion }}</li>
      </ul>
    </template>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PelicanResultDetail } from '@/api/pelicanTests'

// 展开题面与判据时，作品仍在同一详情页中播放。
const props = defineProps<{ sample: PelicanResultDetail; collapsible?: boolean }>()
const { t } = useI18n()
const isProbe = computed(() => props.sample.question_kind === 'state_probe')
</script>
