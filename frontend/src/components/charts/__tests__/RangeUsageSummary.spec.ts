// FORK: 二开用例——区间用量概览卡片
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import RangeUsageSummary from '../RangeUsageSummary.vue'
import type { RangeTotals } from '../rangeTotals'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      // 带上插值参数，测试里才能断言「日均 200.00K」里的数字
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${Object.values(params).join(' ')}` : key
    })
  }
})

const totals: RangeTotals = {
  requests: 1842,
  inputTokens: 30,
  outputTokens: 10,
  cacheCreationTokens: 10,
  cacheReadTokens: 60,
  totalTokens: 1_400_000,
  cost: 5,
  actualCost: 4.21
}

const mountSummary = (overrides: Record<string, unknown> = {}) =>
  mount(RangeUsageSummary, {
    props: { totals, days: 7, rangeLabel: '2026-10-08 ~ 2026-10-14', ...overrides },
    global: { stubs: { Icon: true, LoadingSpinner: true } }
  })

describe('RangeUsageSummary', () => {
  it('renders the four range figures', () => {
    const text = mountSummary().text()

    expect(text).toContain('admin.dashboard.rangeSummary.title')
    expect(text).toContain('admin.dashboard.rangeSummary.totalTokens')
    expect(text).toContain('admin.dashboard.rangeSummary.requests')
    expect(text).toContain('admin.dashboard.rangeSummary.cacheRate')
    expect(text).toContain('admin.dashboard.rangeSummary.actualCost')

    expect(text).toContain('1.40M')
    expect(text).toContain('1,842')
    expect(text).toContain('60.0%')
    expect(text).toContain('$4.21')
  })

  it('shows the range it summarises, so it is not confused with the all-time cards above', () => {
    expect(mountSummary().text()).toContain('2026-10-08 ~ 2026-10-14')
  })

  it('divides by the number of days in the range', () => {
    const wrapper = mountSummary()

    expect(wrapper.text()).toContain('admin.dashboard.rangeSummary.dailyTokens')
    expect(wrapper.text()).toContain('200.00K')
    expect(wrapper.text()).toContain('admin.dashboard.rangeSummary.dailyRequests')
    expect(wrapper.text()).toContain('263')
  })

  it('drops the daily lines for a range that spans no more than a day', () => {
    const text = mountSummary({ days: 1 }).text()

    expect(text).not.toContain('admin.dashboard.rangeSummary.dailyTokens')
    expect(text).not.toContain('admin.dashboard.rangeSummary.dailyRequests')
    expect(text).toContain('1.40M')
  })

  it('shows what the cache saved and the standard price', () => {
    const text = mountSummary().text()

    expect(text).toContain('admin.dashboard.rangeSummary.cacheSaved 60')
    expect(text).toContain('admin.dashboard.rangeSummary.standardCost $5.00')
  })
})
