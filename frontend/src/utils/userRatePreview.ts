// getUserDisplayRateMultiplier 返回账号 233 在分组 8 的测试展示倍率。
export function getUserDisplayRateMultiplier(
  userID: number | null | undefined,
  groupID: number | null | undefined,
  rate: number,
): number {
  if (userID === 233 && groupID === 8 && rate === 0.2) return 0.14
  return rate
}
