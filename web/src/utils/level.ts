// 用户等级计算（与后端 interfaces.CalcLevel 保持一致）
// 阈值：Lv1<50, Lv2<200, Lv3<500, Lv4<1000, Lv5<2500, Lv6(满级)>=2500
const LEVEL_THRESHOLDS = [50, 200, 500, 1000, 2500, 5000]
const LEVEL_MAX = 6

export function calcLevel(experience?: number): number {
  const exp = experience ?? 0
  for (let i = 0; i < LEVEL_THRESHOLDS.length; i++) {
    if (i === LEVEL_MAX - 1) return LEVEL_MAX
    if (exp < LEVEL_THRESHOLDS[i]) return i + 1
  }
  return LEVEL_MAX
}

export function isMaxLevel(level: number): boolean {
  return level >= LEVEL_MAX
}
