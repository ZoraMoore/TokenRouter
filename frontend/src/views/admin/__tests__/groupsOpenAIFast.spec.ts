import { describe, expect, it } from "vitest";
import {
  normalizeGroupOpenAIFastPolicy,
} from "../groupsOpenAIFast";
import en from "@/i18n/locales/en/admin/overview";
import zh from "@/i18n/locales/zh/admin/overview";

describe("groupsOpenAIFast", () => {
  it("分组保存策略，执行时按提供商能力应用", () => {
    expect(normalizeGroupOpenAIFastPolicy('force_ultrafast')).toBe('force_ultrafast');
    expect(normalizeGroupOpenAIFastPolicy('unknown')).toBe('follow_request');
  });

  it("提供免费 Fast 的中英文管理文案", () => {
    expect(zh.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      hint: expect.stringContaining("Ultra Fast"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
    expect(en.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      hint: expect.stringContaining("Ultra Fast"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
  });
});
