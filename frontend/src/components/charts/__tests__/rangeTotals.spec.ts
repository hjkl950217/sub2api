// FORK: 二开用例——仪表盘区间汇总的求和、缓存率与格式化
import { describe, expect, it } from 'vitest'

import type { TrendDataPoint } from '@/types'
import { cacheHitRate, sumTrendTotals } from '../rangeTotals'

const point = (overrides: Partial<TrendDataPoint> = {}): TrendDataPoint => ({
  date: '2026-10-01',
  requests: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  cost: 0,
  actual_cost: 0,
  ...overrides
})

describe('sumTrendTotals', () => {
  it('adds up every field across the buckets', () => {
    const totals = sumTrendTotals([
      point({
        requests: 2,
        input_tokens: 100,
        output_tokens: 10,
        cache_creation_tokens: 5,
        cache_read_tokens: 50,
        total_tokens: 160,
        cost: 0.1,
        actual_cost: 0.08
      }),
      point({
        requests: 3,
        input_tokens: 200,
        output_tokens: 20,
        cache_creation_tokens: 7,
        cache_read_tokens: 90,
        total_tokens: 310,
        cost: 0.2,
        actual_cost: 0.16
      })
    ])

    expect(totals.requests).toBe(5)
    expect(totals.inputTokens).toBe(300)
    expect(totals.outputTokens).toBe(30)
    expect(totals.cacheCreationTokens).toBe(12)
    expect(totals.cacheReadTokens).toBe(140)
    expect(totals.totalTokens).toBe(470)
    expect(totals.cost).toBeCloseTo(0.3)
    expect(totals.actualCost).toBeCloseTo(0.24)
  })

  it('returns zeros when there is no trend data', () => {
    const zero = {
      requests: 0,
      inputTokens: 0,
      outputTokens: 0,
      cacheCreationTokens: 0,
      cacheReadTokens: 0,
      totalTokens: 0,
      cost: 0,
      actualCost: 0
    }

    expect(sumTrendTotals(undefined)).toEqual(zero)
    expect(sumTrendTotals([])).toEqual(zero)
  })
})

describe('cacheHitRate', () => {
  it('is cache reads over all prompt tokens', () => {
    expect(
      cacheHitRate({
        requests: 0,
        inputTokens: 30,
        outputTokens: 0,
        cacheCreationTokens: 10,
        cacheReadTokens: 60,
        totalTokens: 0,
        cost: 0,
        actualCost: 0
      })
    ).toBeCloseTo(60)
  })

  it('is zero when no prompt tokens were spent', () => {
    expect(cacheHitRate(sumTrendTotals([]))).toBe(0)
  })
})
