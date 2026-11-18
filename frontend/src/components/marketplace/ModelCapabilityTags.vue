<template>
  <!-- 模型能力标识：输入模态图标 -> 输出模态图标，每个图标悬停显示模态名称。 -->
  <span
    class="inline-flex min-w-0 max-w-full flex-wrap items-center justify-end gap-1"
    data-testid="model-capability-tags"
  >
    <ModalityIcon
      v-for="modality in capabilities.input"
      :key="`input-${modality}`"
      :data-modality="`input-${modality}`"
      :modality="modality"
    />
    <span v-if="!capabilities.input.length" class="text-xs text-gray-500">{{ t(props.model.attributes?.input_modalities === undefined ? 'admin.modelAttributes.unknown' : 'admin.modelAttributes.none') }}</span>
    <Icon name="moveRight" size="xs" :stroke-width="2" class="text-gray-400 dark:text-dark-500" />
    <ModalityIcon
      v-for="modality in capabilities.output"
      :key="`output-${modality}`"
      :data-modality="`output-${modality}`"
      :modality="modality"
    />
    <span v-if="!capabilities.output.length" class="text-xs text-gray-500">{{ t(props.model.attributes?.output_modalities === undefined ? 'admin.modelAttributes.unknown' : 'admin.modelAttributes.none') }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModalityIcon from '@/components/common/ModalityIcon.vue'
import { resolveModelCapabilities, type ModelModality } from '@/utils/modelCapabilities'
import type { MarketplaceModel } from '@/types'

const props = defineProps<{
  model: Pick<MarketplaceModel, 'id' | 'pricing' | 'input_modalities' | 'output_modalities' | 'attributes'>
}>()

const { t } = useI18n()

// 模型包含 attributes 时读取其模态字段，缺少 attributes 的旧接口响应通过模型信息推断模态。
const capabilities = computed(() =>
  props.model.attributes ? {
    input: (props.model.attributes.input_modalities ?? []) as ModelModality[],
    output: (props.model.attributes.output_modalities ?? []) as ModelModality[],
  } : resolveModelCapabilities(props.model.id, props.model.pricing, {
    input: props.model.input_modalities,
    output: props.model.output_modalities,
  })
)
</script>
