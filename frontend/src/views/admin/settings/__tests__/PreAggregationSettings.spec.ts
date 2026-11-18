import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import PreAggregationSettings from "../PreAggregationSettings.vue";
import {
  createSettingsSaveRegistry,
  settingsSaveRegistryKey,
} from "@/composables/useSettingsSaveRegistry";

const { getSettings, updateSettings, backfill, showError, showSuccess } = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  backfill: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

const response = {
  settings: {
    usage: { enabled: true, interval_seconds: 60 },
    ops: { enabled: false },
  },
  availability: {
    usage_available: true,
    ops_available: true,
    manual_backfill_available: true,
    manual_backfill_max_days: 31,
  },
  usage_status: {
    phase: "idle",
    live_watermark: "2026-08-04T00:00:00Z",
    coverage_start: "2026-08-01T00:00:00Z",
    lag_seconds: 12,
    last_duration_ms: 250,
  },
  ops_status: { phase: "idle", lag_seconds: 0, last_duration_ms: 120 },
};

vi.mock("@/api", () => ({
  adminAPI: {
    settings: {
      getPreAggregationSettings: getSettings,
      updatePreAggregationSettings: updateSettings,
      backfillPreAggregation: backfill,
    },
  },
}));

vi.mock("@/stores", () => ({
  useAppStore: () => ({ showError, showSuccess }),
}));

vi.mock("vue-i18n", () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: "zh-CN" },
  }),
}));

describe("PreAggregationSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getSettings.mockResolvedValue(structuredClone(response));
    updateSettings.mockResolvedValue(structuredClone(response));
    backfill.mockResolvedValue({ status: "accepted", days: 3 });
  });

  // 组件把修改状态和保存函数登记到系统设置页的吸底保存条，测试里直接提供登记表。
  function mountWithSaveRegistry() {
    const saveRegistry = createSettingsSaveRegistry();
    const wrapper = mount(PreAggregationSettings, {
      global: { provide: { [settingsSaveRegistryKey as symbol]: saveRegistry.registry } },
    });
    return { wrapper, saveRegistry };
  }

  it("加载并保存唯一的预聚合运行时配置", async () => {
    const { wrapper, saveRegistry } = mountWithSaveRegistry();
    await flushPromises();
    expect(saveRegistry.dirty.value).toBe(false);

    const switches = wrapper.findAll('button[role="switch"]');
    expect(switches).toHaveLength(2);
    expect(switches[0].attributes("aria-checked")).toBe("true");
    expect(switches[1].attributes("aria-checked")).toBe("false");

    await switches[0].trigger("click");
    await switches[1].trigger("click");
    await wrapper.findAll('input[type="number"]')[0].setValue("120");
    expect(saveRegistry.dirty.value).toBe(true);

    await expect(saveRegistry.saveDirty()).resolves.toBe(true);
    await flushPromises();

    expect(updateSettings).toHaveBeenCalledWith({
      usage: { enabled: false, interval_seconds: 120 },
      ops: { enabled: true },
    });
    expect(saveRegistry.dirty.value).toBe(false);
  });

  it("刷新运行状态时保留未保存的修改", async () => {
    const { wrapper, saveRegistry } = mountWithSaveRegistry();
    await flushPromises();

    await wrapper.findAll('input[type="number"]')[0].setValue("120");
    await wrapper.get(`button[title="common.refresh"]`).trigger("click");
    await flushPromises();

    expect(getSettings).toHaveBeenCalledTimes(2);
    expect((wrapper.findAll('input[type="number"]')[0].element as HTMLInputElement).value).toBe("120");
    expect(saveRegistry.dirty.value).toBe(true);
  });

  it("提交受限天数的异步历史回填", async () => {
    const wrapper = mount(PreAggregationSettings);
    await flushPromises();

    await wrapper.findAll('input[type="number"]')[1].setValue("3");
    const start = wrapper.findAll("button").find((button) => button.text().includes("startBackfill"));
    expect(start).toBeDefined();
    await start!.trigger("click");
    await flushPromises();

    expect(backfill).toHaveBeenCalledWith(3);
    expect(getSettings).toHaveBeenCalledTimes(2);
    expect(showSuccess).toHaveBeenCalled();
  });
});
