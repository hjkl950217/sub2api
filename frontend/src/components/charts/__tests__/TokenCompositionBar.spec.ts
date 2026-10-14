// FORK: 二开用例——Token 构成占比卡片
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import TokenCompositionBar from '../TokenCompositionBar.vue'
import type { RangeTotals } from '../rangeTotals'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

// 缓存读占了七成——自用场景下的典型形态
const totals: RangeTotals = {
  requests: 100,
  inputTokens: 100_000,
  outputTokens: 200_000,
  cacheCreationTokens: 100_000,
  cacheReadTokens: 1_000_000,
  totalTokens: 1_400_000,
  cost: 5,
  actualCost: 4.21
}

const mountBar = (overrides: Record<string, unknown> = {}) =>
  mount(TokenCompositionBar, {
    props: { totals, ...overrides },
    global: { stubs: { LoadingSpinner: true } }
  })

describe('TokenCompositionBar', () => {
  it('lists the four token buckets with their share of the total', () => {
    const text = mountBar().text()

    expect(text).toContain('admin.dashboard.tokenComposition.title')
    expect(text).toContain('admin.dashboard.tokenComposition.input')
    expect(text).toContain('admin.dashboard.tokenComposition.output')
    expect(text).toContain('admin.dashboard.tokenComposition.cacheCreation')
    expect(text).toContain('admin.dashboard.tokenComposition.cacheRead')

    expect(text).toContain('100.00万')
    expect(text).toContain('71.4%')
    expect(text).toContain('14.3%')
  })

  it('puts the biggest bucket first, sizing every segment by its share', () => {
    const segments = mountBar().findAll('[data-composition-segment]')

    expect(segments.map(segment => segment.attributes('data-composition-segment'))).toEqual([
      'cacheRead',
      'output',
      'input',
      'cacheCreation'
    ])
    expect(segments.map(segment => segment.element.style.width)).toEqual([
      '71.4%',
      '14.3%',
      '7.1%',
      '7.1%'
    ])
  })

  it('keeps equal buckets in their original order', () => {
    const wrapper = mountBar({
      totals: {
        requests: 1,
        inputTokens: 40,
        outputTokens: 60,
        cacheCreationTokens: 40,
        cacheReadTokens: 60,
        totalTokens: 200,
        cost: 0,
        actualCost: 0
      }
    })

    // output 和 cacheRead 并列、input 和 cacheCreation 并列，按清单顺序稳定排
    expect(
      wrapper.findAll('[data-composition-segment]').map(s => s.attributes('data-composition-segment'))
    ).toEqual(['output', 'cacheRead', 'input', 'cacheCreation'])
  })

  it('shows the empty state instead of a bar when nothing was used', () => {
    const wrapper = mountBar({
      totals: {
        requests: 0,
        inputTokens: 0,
        outputTokens: 0,
        cacheCreationTokens: 0,
        cacheReadTokens: 0,
        totalTokens: 0,
        cost: 0,
        actualCost: 0
      }
    })

    expect(wrapper.findAll('[data-composition-segment]')).toHaveLength(0)
    expect(wrapper.text()).toContain('admin.dashboard.noDataAvailable')
  })
})
