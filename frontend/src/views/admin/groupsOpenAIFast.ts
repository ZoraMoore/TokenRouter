import type { GroupOpenAIFastPolicy } from "@/types";

// 策略保存在分组，执行时由适用的提供商和协议使用。
export function normalizeGroupOpenAIFastPolicy(policy: string): GroupOpenAIFastPolicy {
  if (["follow_request", "force_priority", "force_ultrafast", "force_off"].includes(policy)) {
    return policy as GroupOpenAIFastPolicy;
  }
  return "follow_request";
}
