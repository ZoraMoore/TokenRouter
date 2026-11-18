<template>
  <SettingsSection>
    <SettingToggleRow
      :id="`${idPrefix}-model-routing`"
      :model-value="enabled"
      :label="t('admin.groups.modelRouting.title')"
      :hint="
        t(
          enabled
            ? 'admin.groups.modelRouting.noRulesHint'
            : 'admin.groups.modelRouting.disabledHint',
        )
      "
      :help="t('admin.groups.modelRouting.tooltip')"
      setting="model_routing_enabled"
      @update:model-value="emit('enabled', $event)"
    />
    <RuleListEditor
      v-if="enabled"
      :items="rules"
      :item-key="getKey"
      variant="card"
      :item-label="(index) => t('admin.groups.modelRouting.ruleIndex', { index: index + 1 })"
      :add-label="t('admin.groups.modelRouting.addRule')"
      add-placement="footer"
      :empty-text="t('admin.groups.modelRouting.noRules')"
      :remove-label="t('admin.groups.modelRouting.removeRule')"
      @add="emit('add')"
      @remove="(index) => emit('remove', props.rules[index])"
    >
      <template #row="{ item: rule }">
        <div class="space-y-4">
          <div>
            <label :for="`${getKey(rule)}-pattern`" class="input-label">{{
              t('admin.groups.modelRouting.modelPattern')
            }}</label>
            <input
              :id="`${getKey(rule)}-pattern`"
              :value="rule.pattern"
              type="text"
              class="input"
              :placeholder="
                t('admin.groups.modelRouting.modelPatternPlaceholder')
              "
              @input="
                emit('pattern', rule, ($event.target as HTMLInputElement).value)
              "
            />
          </div>
          <div>
            <label :for="`${getKey(rule)}-providers`" class="input-label">{{
              t('admin.groups.modelRouting.providers')
            }}</label>
            <div v-if="rule.providers.length" class="mb-2 flex flex-wrap gap-2">
              <span
                v-for="provider in rule.providers"
                :key="provider.id"
                class="inline-flex max-w-full items-center gap-2 rounded-compact bg-primary-100 px-2 py-1 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
              >
                <span class="min-w-0 break-all">{{ provider.name }}</span>
                <button
                  type="button"
                  class="shrink-0 rounded-compact focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
                  :aria-label="
                    t('admin.groups.settings.removeItem', { name: provider.name })
                  "
                  @click="emit('removeProvider', rule, provider.id)"
                >
                  <Icon name="x" size="sm" />
                </button>
              </span>
            </div>
            <div class="relative provider-search-container">
              <input
                :id="`${getKey(rule)}-providers`"
                :value="search.keywords[getKey(rule)] || ''"
                type="text"
                class="input"
                :placeholder="
                  t('admin.groups.modelRouting.searchProviderPlaceholder')
                "
                @input="
                  emit('search', rule, ($event.target as HTMLInputElement).value)
                "
                @focus="emit('focus', rule)"
              />
              <MotionTransition name="dropdown-fade">
                <div
                  v-if="
                    search.open[getKey(rule)] &&
                    search.results[getKey(rule)]?.length
                  "
                  class="dropdown absolute left-0 right-0 z-50 mt-1 max-h-menu-sm overflow-auto"
                >
                  <button
                    v-for="provider in search.results[getKey(rule)]"
                    :key="provider.id"
                    type="button"
                    class="dropdown-item-sm"
                    :disabled="
                      rule.providers.some((selected) => selected.id === provider.id)
                    "
                    :class="{
                      'opacity-50': rule.providers.some(
                        (selected) => selected.id === provider.id,
                      ),
                    }"
                    @click="emit('selectProvider', rule, provider)"
                  >
                    <span class="min-w-0 break-all">{{ provider.name }}</span>
                    <span class="shrink-0 text-xs text-gray-400"
                      >#{{ provider.id }}</span
                    >
                  </button>
                </div>
              </MotionTransition>
            </div>
            <p class="input-hint">
              {{ t('admin.groups.modelRouting.providersHint') }}
            </p>
          </div>
        </div>
      </template>
    </RuleListEditor>
  </SettingsSection>
</template>

<script setup lang="ts">
import MotionTransition from '@/components/common/MotionTransition.vue'

import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import RuleListEditor from '@/components/common/RuleListEditor.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import type {
  GroupProviderSearchState,
  GroupModelRoutingRule,
  GroupRoutingProvider,
} from './groupSettingsTypes'

// 复用规则对象，页面的搜索键和取消请求逻辑通过对象身份定位同一条规则。
const props = defineProps<{
  idPrefix: string
  enabled: boolean
  rules: GroupModelRoutingRule[]
  search: GroupProviderSearchState
  getKey: (rule: GroupModelRoutingRule) => string
}>()
const emit = defineEmits<{
  enabled: [value: boolean]
  add: []
  remove: [rule: GroupModelRoutingRule]
  pattern: [rule: GroupModelRoutingRule, value: string]
  search: [rule: GroupModelRoutingRule, keyword: string]
  focus: [rule: GroupModelRoutingRule]
  selectProvider: [rule: GroupModelRoutingRule, provider: GroupRoutingProvider]
  removeProvider: [rule: GroupModelRoutingRule, providerId: number]
}>()
const { t } = useI18n()
</script>
