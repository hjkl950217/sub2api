// FORK: 二开文件——仪表盘区间汇总。
//
// 仪表盘顶部的统计卡片来自 `GetDashboardStats(ctx)`，不随时间范围变化（累计 / 今日）。
// 区间数据只能从 `trend`（区间时序）或 `models`（区间按模型）求和得到，两者字段一致。
// 这里集中放求和、缓存率与格式化，供「区间用量概览」「Token 构成占比」共用。
import type { TrendDataPoint } from '@/types'

export interface RangeTotals {
  requests: number
  inputTokens: number
  outputTokens: number
  cacheCreationTokens: number
  cacheReadTokens: number
  totalTokens: number
  cost: number
  actualCost: number
}

const EMPTY_TOTALS: RangeTotals = {
  requests: 0,
  inputTokens: 0,
  outputTokens: 0,
  cacheCreationTokens: 0,
  cacheReadTokens: 0,
  totalTokens: 0,
  cost: 0,
  actualCost: 0
}

export function sumTrendTotals(trend: TrendDataPoint[] | null | undefined): RangeTotals {
  if (!trend?.length) return { ...EMPTY_TOTALS }

  return trend.reduce<RangeTotals>(
    (acc, point) => ({
      requests: acc.requests + (point.requests || 0),
      inputTokens: acc.inputTokens + (point.input_tokens || 0),
      outputTokens: acc.outputTokens + (point.output_tokens || 0),
      cacheCreationTokens: acc.cacheCreationTokens + (point.cache_creation_tokens || 0),
      cacheReadTokens: acc.cacheReadTokens + (point.cache_read_tokens || 0),
      totalTokens: acc.totalTokens + (point.total_tokens || 0),
      cost: acc.cost + (point.cost || 0),
      actualCost: acc.actualCost + (point.actual_cost || 0)
    }),
    { ...EMPTY_TOTALS }
  )
}

// 缓存命中率口径与 TokenUsageTrend.vue 一致：缓存读 / 全部提示词 token。
// input_tokens 不含缓存部分，所以分母要把两段缓存都加上。
export function cacheHitRate(totals: RangeTotals): number {
  const promptTokens = totals.inputTokens + totals.cacheReadTokens + totals.cacheCreationTokens
  if (promptTokens <= 0) return 0
  return (totals.cacheReadTokens / promptTokens) * 100
}

// 与仪表盘统计卡片的 formatTokens 同一套档位，保证两处数字长度一致
export function formatRangeTokens(value: number | null | undefined): string {
  const safe = Number(value)
  if (!Number.isFinite(safe) || safe === 0) return '0'
  if (safe >= 1_000_000_000) return `${(safe / 1_000_000_000).toFixed(2)}B`
  if (safe >= 1_000_000) return `${(safe / 1_000_000).toFixed(2)}M`
  if (safe >= 1_000) return `${(safe / 1_000).toFixed(2)}K`
  return safe.toLocaleString()
}

// 同上，对应仪表盘的 formatCost
export function formatRangeCost(value: number | null | undefined): string {
  const safe = Number(value)
  if (!Number.isFinite(safe)) return '$0.0000'
  if (safe >= 1000) return `$${(safe / 1000).toFixed(2)}K`
  if (safe >= 1) return `$${safe.toFixed(2)}`
  if (safe >= 0.01) return `$${safe.toFixed(3)}`
  return `$${safe.toFixed(4)}`
}
