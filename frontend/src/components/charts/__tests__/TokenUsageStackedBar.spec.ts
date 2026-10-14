// FORK: 二开用例——仪表盘的 Token 使用趋势改成的堆叠柱状图
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import type { TrendDataPoint } from '@/types'
import TokenUsageStackedBar from '../TokenUsageStackedBar.vue'

vi.mock('vue-chartjs', () => ({
  Bar: { name: 'Bar', props: ['data', 'options'], template: '<div class="bar-chart" />' }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const point = (overrides: Partial<TrendDataPoint> = {}): TrendDataPoint => ({
  date: '2026-10-01',
  requests: 1,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  cost: 0,
  actual_cost: 0,
  ...overrides
})

const mountBar = (trendData: TrendDataPoint[], loading = false) =>
  mount(TokenUsageStackedBar, {
    props: { trendData, loading },
    global: { stubs: { LoadingSpinner: true } }
  })

describe('TokenUsageStackedBar', () => {
  it('breaks every period into the four token buckets', () => {
    const bar = mountBar([
      point({ date: '2026-10-01', input_tokens: 10, output_tokens: 1, cache_creation_tokens: 2, cache_read_tokens: 3 }),
      point({ date: '2026-10-02', input_tokens: 30, output_tokens: 4, cache_creation_tokens: 5, cache_read_tokens: 6 })
    ]).findComponent({ name: 'Bar' })

    expect(bar.props('data').labels).toEqual(['2026-10-01', '2026-10-02'])

    const datasets = bar.props('data').datasets
    expect(datasets.map((set: { label: string }) => set.label)).toEqual([
      'admin.dashboard.tokenComposition.input',
      'admin.dashboard.tokenComposition.output',
      'admin.dashboard.tokenComposition.cacheCreation',
      'admin.dashboard.tokenComposition.cacheRead'
    ])
    expect(datasets[0].data).toEqual([10, 30])
    expect(datasets[3].data).toEqual([3, 6])
  })

  it('stacks the buckets instead of placing them side by side', () => {
    const bar = mountBar([point({ input_tokens: 10 })]).findComponent({ name: 'Bar' })

    const datasets = bar.props('data').datasets
    expect(datasets.every((set: { stack?: string }) => set.stack === 'tokens')).toBe(true)

    const options = bar.props('options')
    expect(options.scales.x.stacked).toBe(true)
    expect(options.scales.y.stacked).toBe(true)
  })

  it('drops the percentage axis that made the old line chart hard to read', () => {
    const bar = mountBar([point({ input_tokens: 10 })]).findComponent({ name: 'Bar' })

    expect(bar.props('options').scales.yPercent).toBeUndefined()
  })

  it('shows the empty state when the range has no points', () => {
    const wrapper = mountBar([])

    expect(wrapper.findComponent({ name: 'Bar' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.dashboard.noDataAvailable')
  })
})
