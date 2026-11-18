<template>
  <SettingsCard
    :title="t('admin.settings.preAggregation.title')"
    :description="t('admin.settings.preAggregation.description')"
  >
    <template #actions>
      <button
        type="button"
        class="btn btn-secondary btn-icon shrink-0"
        :disabled="loading"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="loadSettings"
      >
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
      </button>
    </template>

    <ContentSkeleton v-if="loading && !state" variant="form" :rows="4" />

    <template v-else-if="state">
      <SettingsSection>
        <SettingToggleRow
          :id="`${uid}-usage-enabled`"
          v-model="form.usage.enabled"
          :label="t('admin.settings.preAggregation.usage')"
          :disabled="!state.availability.usage_available || saving"
        >
          <template v-if="!state.availability.usage_available" #hint>
            <SettingsNotice tone="warning" class="mt-2">
              {{ t("admin.settings.preAggregation.unavailable") }}
            </SettingsNotice>
          </template>
        </SettingToggleRow>
        <SettingRow
          :id="`${uid}-usage-interval`"
          field
          :label-for="`${uid}-usage-interval`"
          :label="t('admin.settings.preAggregation.interval')"
        >
          <input
            :id="`${uid}-usage-interval`"
            v-model.number="form.usage.interval_seconds"
            type="number"
            min="30"
            max="3600"
            step="30"
            class="input"
            :disabled="saving"
          />
        </SettingRow>
        <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-sm xl:grid-cols-4">
          <StatusItem :label="t('admin.settings.preAggregation.phase')">
            <span :class="phaseClass(state.usage_status.phase)">{{ phaseLabel(state.usage_status.phase) }}</span>
          </StatusItem>
          <StatusItem :label="t('admin.settings.preAggregation.coverage')" class="col-span-2 xl:col-span-1">
            {{ usageCoverage }}
          </StatusItem>
          <StatusItem :label="t('admin.settings.preAggregation.lastSuccess')">
            {{ formatDate(state.usage_status.last_success_at) }}
          </StatusItem>
          <StatusItem :label="t('admin.settings.preAggregation.lag')">
            {{ formatDuration(state.usage_status.lag_seconds * 1000) }}
          </StatusItem>
          <StatusItem :label="t('admin.settings.preAggregation.lastDuration')">
            {{ formatDuration(state.usage_status.last_duration_ms) }}
          </StatusItem>
          <StatusItem
            v-if="state.usage_status.last_error"
            :label="t('admin.settings.preAggregation.lastError')"
            class="col-span-2 text-red-600 dark:text-red-400 xl:col-span-3"
          >
            {{ state.usage_status.last_error }}
          </StatusItem>
        </dl>
      </SettingsSection>

      <SettingsSection>
        <SettingToggleRow
          :id="`${uid}-ops-enabled`"
          v-model="form.ops.enabled"
          :label="t('admin.settings.preAggregation.ops')"
          :disabled="!state.availability.ops_available || saving"
        >
          <template v-if="!state.availability.ops_available" #hint>
            <SettingsNotice tone="warning" class="mt-2">
              {{ t("admin.settings.preAggregation.unavailable") }}
            </SettingsNotice>
          </template>
        </SettingToggleRow>
        <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-sm xl:grid-cols-4">
          <StatusItem :label="t('admin.settings.preAggregation.phase')">
            <span :class="phaseClass(state.ops_status.phase)">{{ phaseLabel(state.ops_status.phase) }}</span>
          </StatusItem>
          <StatusItem :label="t('admin.settings.preAggregation.lastSuccess')">
            {{ formatDate(state.ops_status.last_success_at) }}
          </StatusItem>
          <StatusItem :label="t('admin.settings.preAggregation.lastDuration')">
            {{ formatDuration(state.ops_status.last_duration_ms) }}
          </StatusItem>
          <StatusItem
            v-if="state.ops_status.last_error"
            :label="t('admin.settings.preAggregation.lastError')"
            class="col-span-2 text-red-600 dark:text-red-400 xl:col-span-1"
          >
            {{ state.ops_status.last_error }}
          </StatusItem>
        </dl>
      </SettingsSection>

      <SettingsSection>
        <SettingRow
          :id="`${uid}-backfill-days`"
          field
          :label-for="`${uid}-backfill-days`"
          :label="t('admin.settings.preAggregation.backfillDays')"
        >
          <div class="flex gap-2">
            <input
              :id="`${uid}-backfill-days`"
              v-model.number="backfillDays"
              type="number"
              min="1"
              :max="state.availability.manual_backfill_max_days"
              class="input"
              :disabled="backfilling || !canBackfill"
            />
            <button
              type="button"
              class="btn btn-secondary shrink-0"
              :disabled="backfilling || !canBackfill"
              @click="startBackfill"
            >
              <Icon
                :name="backfilling ? 'refresh' : 'play'"
                size="sm"
                :class="backfilling ? 'animate-spin' : ''"
              />
              {{ t("admin.settings.preAggregation.startBackfill") }}
            </button>
          </div>
        </SettingRow>
      </SettingsSection>
    </template>
  </SettingsCard>
</template>

<script setup lang="ts">
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import { computed, defineComponent, h, nextTick, onMounted, reactive, ref, useId } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type { PreAggregationSettingsResponse } from "@/api/admin/settings";
import SettingRow from "@/components/common/settings/SettingRow.vue";
import SettingToggleRow from "@/components/common/settings/SettingToggleRow.vue";
import SettingsCard from "@/components/common/settings/SettingsCard.vue";
import SettingsNotice from "@/components/common/settings/SettingsNotice.vue";
import SettingsSection from "@/components/common/settings/SettingsSection.vue";
import Icon from "@/components/icons/Icon.vue";
import { useDirtyTracker } from "@/composables/useDirtyTracker";
import { useSettingsSaveTarget } from "@/composables/useSettingsSaveRegistry";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";

// 状态项在设置卡片内使用无边框网格。
const StatusItem = defineComponent({
  props: { label: { type: String, required: true } },
  setup(props, { slots, attrs }) {
    return () => h("div", attrs, [
      h("dt", { class: "text-xs text-gray-500 dark:text-dark-400" }, props.label),
      h("dd", { class: "mt-1 break-words font-medium text-gray-800 dark:text-dark-100" }, slots.default?.()),
    ]);
  },
});

const { t, locale } = useI18n();
const uid = useId();
const appStore = useAppStore();
const loading = ref(false);
const saving = ref(false);
const backfilling = ref(false);
const state = ref<PreAggregationSettingsResponse | null>(null);
const backfillDays = ref(7);
const form = reactive({
  usage: { enabled: false, interval_seconds: 60 },
  ops: { enabled: false },
});

// 吸底保存条比较开关和间隔与上次加载或保存时的快照。
const { dirty, markClean } = useDirtyTracker({ form: () => form });
useSettingsSaveTarget("preAggregation", {
  dirty,
  save: saveSettings,
  // 先清掉修改状态，重新加载时才会用服务端的值覆盖表单。
  discard: async () => {
    markClean();
    await loadSettings();
  },
});

const canBackfill = computed(() => Boolean(
  state.value?.availability.manual_backfill_available && form.usage.enabled,
));

const usageCoverage = computed(() => {
  const status = state.value?.usage_status;
  if (!status?.coverage_start || !status.live_watermark) {
    return t("admin.settings.preAggregation.noData");
  }
  return `${formatDate(status.coverage_start)} - ${formatDate(status.live_watermark)}`;
});

// 刷新运行状态时，表单里还没保存的修改保持不变；表单与服务端一致时才同步开关和间隔。
function applyResponse(response: PreAggregationSettingsResponse) {
  state.value = response;
  backfillDays.value = Math.min(Math.max(backfillDays.value, 1), response.availability.manual_backfill_max_days);
  if (dirty.value) return;
  form.usage.enabled = response.settings.usage.enabled;
  form.usage.interval_seconds = response.settings.usage.interval_seconds;
  form.ops.enabled = response.settings.ops.enabled;
}

async function loadSettings() {
  loading.value = true;
  const wasDirty = dirty.value;
  try {
    applyResponse(await adminAPI.settings.getPreAggregationSettings());
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("admin.settings.preAggregation.loadFailed")));
  } finally {
    loading.value = false;
  }
  if (!wasDirty) {
    await nextTick();
    markClean();
  }
}

// saveSettings 提交开关和间隔，返回是否保存成功；由吸底保存条统一调用。
async function saveSettings(): Promise<boolean> {
  saving.value = true;
  try {
    const response = await adminAPI.settings.updatePreAggregationSettings({
      usage: {
        enabled: form.usage.enabled,
        interval_seconds: Number(form.usage.interval_seconds),
      },
      ops: { enabled: form.ops.enabled },
    });
    // 先清掉修改状态，applyResponse 才会用服务端返回的值同步表单。
    markClean();
    applyResponse(response);
    await nextTick();
    markClean();
    return true;
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("admin.settings.preAggregation.saveFailed")));
    return false;
  } finally {
    saving.value = false;
  }
}

async function startBackfill() {
  if (!state.value) return;
  backfilling.value = true;
  try {
    await adminAPI.settings.backfillPreAggregation(Number(backfillDays.value));
    appStore.showSuccess(t("admin.settings.preAggregation.backfillAccepted"));
    await loadSettings();
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("admin.settings.preAggregation.backfillFailed")));
  } finally {
    backfilling.value = false;
  }
}

function phaseLabel(phase: string): string {
  const key = `admin.settings.preAggregation.phases.${phase || "unavailable"}`;
  const translated = t(key);
  return translated === key ? phase : translated;
}

function phaseClass(phase: string): string {
  if (phase === "error") return "text-red-600 dark:text-red-400";
  if (phase === "live" || phase === "backfill") return "text-emerald-600 dark:text-emerald-400";
  if (phase === "disabled" || phase === "unavailable") return "text-gray-500 dark:text-gray-400";
  return "text-gray-800 dark:text-gray-200";
}

function formatDate(value?: string): string {
  if (!value) return t("admin.settings.preAggregation.noData");
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale.value, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function formatDuration(milliseconds: number): string {
  if (!Number.isFinite(milliseconds) || milliseconds <= 0) return "0s";
  if (milliseconds < 1000) return `${Math.round(milliseconds)}ms`;
  if (milliseconds < 60_000) return `${(milliseconds / 1000).toFixed(1)}s`;
  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.floor((milliseconds % 60_000) / 1000);
  return `${minutes}m ${seconds}s`;
}

onMounted(loadSettings);
</script>
