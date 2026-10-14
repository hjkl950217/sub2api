import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import type { DashboardStats } from '@/types'
import DashboardView from '../DashboardView.vue'
import RangeUsageSummary from '@/components/charts/RangeUsageSummary.vue'
import TokenCompositionBar from '@/components/charts/TokenCompositionBar.vue'
import TokenUsageStackedBar from '@/components/charts/TokenUsageStackedBar.vue'

const { getSnapshotV2, getUserUsageTrend, getUserSpendingRanking } = vi.hoisted(() => ({
  getSnapshotV2: vi.fn(),
  getUserUsageTrend: vi.fn(),
  getUserSpendingRanking: vi.fn()
}))

vi.mock('vue-chartjs', () => ({
  Line: { name: 'Line', props: ['data', 'options'], template: '<div class="line-chart" />' },
  Bar: { name: 'Bar', props: ['data', 'options'], template: '<div class="bar-chart" />' }
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getSnapshotV2,
      getUserUsageTrend,
      getUserSpendingRanking
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const createDashboardStats = (): DashboardStats => ({
  total_users: 0,
  today_new_users: 0,
  active_users: 0,
  hourly_active_users: 0,
  stats_updated_at: '',
  stats_stale: false,
  total_api_keys: 0,
  active_api_keys: 0,
  total_accounts: 0,
  normal_accounts: 0,
  error_accounts: 0,
  ratelimit_accounts: 0,
  overload_accounts: 0,
  total_requests: 0,
  total_input_tokens: 0,
  total_output_tokens: 0,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 0,
  total_cost: 0,
  total_actual_cost: 0,
  today_requests: 0,
  today_input_tokens: 0,
  today_output_tokens: 0,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 0,
  today_cost: 0,
  today_actual_cost: 0,
  average_duration_ms: 0,
  uptime: 0,
  rpm: 0,
  tpm: 0
})

describe('admin DashboardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()

    getSnapshotV2.mockReset()
    getUserUsageTrend.mockReset()
    getUserSpendingRanking.mockReset()

    getSnapshotV2.mockResolvedValue({
      stats: createDashboardStats(),
      trend: [],
      models: []
    })
    getUserUsageTrend.mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'hour'
    })
    getUserSpendingRanking.mockResolvedValue({
      ranking: [],
      total_actual_cost: 0,
      total_requests: 0,
      total_tokens: 0,
      start_date: '',
      end_date: ''
    })
  })

  it('switches metrics and ignores a stale tokens response', async () => {
    let resolveTokens!: (value: any) => void
    getUserUsageTrend.mockImplementationOnce(() => new Promise(resolve => { resolveTokens = resolve }))
    getUserUsageTrend.mockResolvedValueOnce({ trend: [{ date: '2026-01-01', user_id: 2, username: 'spender', email: '', requests: 1, tokens: 10, cost: 5, actual_cost: 5 }] })
    const wrapper = mount(DashboardView, {
      global: { stubs: {
        AppLayout: { template: '<div><slot /></div>' }, LoadingSpinner: true,
        Icon: true, DateRangePicker: true, Select: true, ModelDistributionChart: true,
        TokenUsageTrend: true,
        BaseDialog: { name: 'BaseDialog', props: { show: Boolean, title: String }, template: '<div v-if="show"><slot /></div>' }
      } }
    })
    await flushPromises()
    // 最近使用默认隐藏，先打开它——这个用例考的是它内部的指标切换与过期响应保护
    await wrapper.findAll('button').find(button => button.text() === 'admin.dashboard.chartSettings.title')!.trigger('click')
    await wrapper.find('[data-chart-row="recentUsage"] [role="switch"]').trigger('click')
    await flushPromises()
    expect(getUserUsageTrend).toHaveBeenCalledWith(expect.objectContaining({ metric: 'tokens', limit: 12 }))
    await wrapper.findAll('button').find(button => button.text() === 'admin.dashboard.actualSpending')!.trigger('click')
    expect(getUserUsageTrend).toHaveBeenCalledWith(expect.objectContaining({ metric: 'actual_cost', limit: 12 }))
    await flushPromises()
    const chart = wrapper.findComponent({ name: 'Line' })
    expect(chart.props('data').datasets[0].data).toEqual([5])
    expect(chart.props('options').scales.y.ticks.callback(5)).toBe('$5.00')
    resolveTokens({ trend: [{ date: '2026-01-01', user_id: 1, username: 'tokens', email: '', requests: 1, tokens: 100, cost: 1, actual_cost: 1 }] })
    await flushPromises()
    expect(chart.props('data').datasets[0].data).toEqual([5])
  })

  it('uses last 24 hours as default dashboard range', async () => {
    mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          Line: true
        }
      }
    })

    await flushPromises()

    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))
  })

  // FORK-ANCHOR: test-dashboard-chart-settings (二开：仪表盘图表开关与区间统计卡片)
  describe('chart settings', () => {
    const chartSettingsLabel = 'admin.dashboard.chartSettings.title'
    const chartItemKey = (id: string, field: string) =>
      `admin.dashboard.chartSettings.items.${id}.${field}`

    const BaseDialogStub = {
      name: 'BaseDialog',
      props: { show: Boolean, title: String },
      template: '<div v-if="show" class="dialog"><slot /></div>'
    }

    // 三张二开卡片不 stub，直接渲染，这样能断言真实数字
    const mountDashboard = () =>
      mount(DashboardView, {
        global: {
          stubs: {
            AppLayout: { template: '<div><slot /></div>' },
            LoadingSpinner: true,
            Icon: true,
            DateRangePicker: true,
            Select: true,
            ModelDistributionChart: true,
            TokenUsageTrend: true,
            Line: true,
            BaseDialog: BaseDialogStub
          }
        }
      })

    const openChartSettings = async (wrapper: ReturnType<typeof mountDashboard>) => {
      const button = wrapper.findAll('button').find(item => item.text() === chartSettingsLabel)!
      await button.trigger('click')
      await flushPromises()
    }

    const toggleChart = async (wrapper: ReturnType<typeof mountDashboard>, id: string) => {
      const row = wrapper.find(`[data-chart-row="${id}"]`)
      await row.find('[role="switch"]').trigger('click')
      await flushPromises()
    }

    const refresh = async (wrapper: ReturnType<typeof mountDashboard>) => {
      const button = wrapper.findAll('button').find(item => item.text() === 'common.refresh')!
      await button.trigger('click')
      await flushPromises()
    }

    beforeEach(() => {
      localStorage.clear()
    })

    it('shows the two range cards and the model chart by default, and skips the two charts that are off', async () => {
      const wrapper = mountDashboard()
      await flushPromises()

      expect(wrapper.findComponent(RangeUsageSummary).exists()).toBe(true)
      expect(wrapper.findComponent(TokenCompositionBar).exists()).toBe(true)
      expect(wrapper.findComponent({ name: 'ModelDistributionChart' }).exists()).toBe(true)
      expect(wrapper.findComponent(TokenUsageStackedBar).exists()).toBe(false)
      expect(wrapper.find('[data-testid="recent-usage-chart"]').exists()).toBe(false)

      // 区间概览与构成占比都要 trend，所以 include_trend 仍是 true
      expect(getSnapshotV2).toHaveBeenCalledWith(
        expect.objectContaining({ include_model_stats: true, include_trend: true })
      )
      // 模型分布开着，它的消费排行视图要数据；最近使用关着，不该请求
      expect(getUserSpendingRanking).toHaveBeenCalledTimes(1)
      expect(getUserUsageTrend).not.toHaveBeenCalled()
    })

    it('sums the range trend into the range summary card', async () => {
      getSnapshotV2.mockResolvedValueOnce({
        stats: createDashboardStats(),
        trend: [
          {
            date: '2026-10-13',
            requests: 2,
            input_tokens: 30,
            output_tokens: 10,
            cache_creation_tokens: 10,
            cache_read_tokens: 60,
            total_tokens: 110,
            cost: 0.2,
            actual_cost: 0.16
          },
          {
            date: '2026-10-14',
            requests: 3,
            input_tokens: 70,
            output_tokens: 20,
            cache_creation_tokens: 20,
            cache_read_tokens: 140,
            total_tokens: 250,
            cost: 0.3,
            actual_cost: 0.25
          }
        ],
        models: []
      })

      const wrapper = mountDashboard()
      await flushPromises()

      const summary = wrapper.findComponent(RangeUsageSummary)
      expect(summary.props('totals')).toMatchObject({
        requests: 5,
        inputTokens: 100,
        outputTokens: 30,
        cacheCreationTokens: 30,
        cacheReadTokens: 200,
        totalTokens: 360
      })
      // 200 / (100 + 200 + 30)
      expect(summary.text()).toContain('60.6%')
      expect(summary.text()).toContain('360')
    })

    it('switches the summary to daily averages once the range spans more than a day', async () => {
      const wrapper = mountDashboard()
      await flushPromises()
      expect(wrapper.findComponent(RangeUsageSummary).props('days')).toBe(1)

      const picker = wrapper.findComponent({ name: 'DateRangePicker' })
      picker.vm.$emit('update:startDate', '2026-10-08')
      picker.vm.$emit('update:endDate', '2026-10-14')
      picker.vm.$emit('change', { startDate: '2026-10-08', endDate: '2026-10-14', preset: null })
      await flushPromises()

      expect(wrapper.findComponent(RangeUsageSummary).props('days')).toBe(7)
    })

    it('lists every chart in the settings dialog, with the switch left of the name', async () => {
      const wrapper = mountDashboard()
      await flushPromises()

      expect(wrapper.find('.dialog').exists()).toBe(false)

      await openChartSettings(wrapper)

      expect(wrapper.findComponent(BaseDialogStub).props('show')).toBe(true)

      const rows = wrapper.findAll('[data-chart-row]')
      expect(rows.map(row => row.attributes('data-chart-row'))).toEqual([
        'rangeSummary',
        'modelDistribution',
        'tokenComposition',
        'tokenUsageTrend',
        'recentUsage'
      ])

      for (const row of rows) {
        const id = row.attributes('data-chart-row')!
        const switchElement = row.find('[role="switch"]')
        expect(switchElement.exists()).toBe(true)
        expect(row.element.firstElementChild).toBe(switchElement.element)
        expect(row.text()).toContain(chartItemKey(id, 'name'))
        expect(row.text()).toContain(chartItemKey(id, 'desc'))
      }
    })

    it('renders a chart and loads its data as soon as its switch is turned on', async () => {
      const wrapper = mountDashboard()
      await flushPromises()
      expect(getUserUsageTrend).not.toHaveBeenCalled()

      await openChartSettings(wrapper)
      await toggleChart(wrapper, 'recentUsage')

      expect(wrapper.find('[data-testid="recent-usage-chart"]').exists()).toBe(true)
      expect(getUserUsageTrend).toHaveBeenCalledTimes(1)
    })

    it('stops rendering and stops requesting a chart once its switch is turned off', async () => {
      const wrapper = mountDashboard()
      await flushPromises()

      await openChartSettings(wrapper)
      await toggleChart(wrapper, 'modelDistribution')
      expect(wrapper.findComponent({ name: 'ModelDistributionChart' }).exists()).toBe(false)

      getSnapshotV2.mockClear()
      await refresh(wrapper)

      expect(getSnapshotV2).toHaveBeenCalledWith(
        expect.objectContaining({ include_model_stats: false, include_trend: true })
      )
      expect(getUserSpendingRanking).toHaveBeenCalledTimes(1)
    })

    it('stops asking for trend data once every chart that needs it is turned off', async () => {
      const wrapper = mountDashboard()
      await flushPromises()

      await openChartSettings(wrapper)
      await toggleChart(wrapper, 'rangeSummary')
      await toggleChart(wrapper, 'tokenComposition')
      await toggleChart(wrapper, 'modelDistribution')

      getSnapshotV2.mockClear()
      await refresh(wrapper)

      expect(getSnapshotV2).toHaveBeenCalledWith(
        expect.objectContaining({ include_trend: false, include_model_stats: false })
      )
    })

    it('remembers the toggled charts on the next visit', async () => {
      const first = mountDashboard()
      await flushPromises()
      await openChartSettings(first)
      await toggleChart(first, 'tokenUsageTrend')
      first.unmount()

      const second = mountDashboard()
      await flushPromises()

      expect(second.findComponent(TokenUsageStackedBar).exists()).toBe(true)
      expect(getSnapshotV2).toHaveBeenLastCalledWith(
        expect.objectContaining({ include_trend: true, include_model_stats: true })
      )
    })

    it('keeps a chart added later at its own default for someone who saved settings before', async () => {
      // 旧格式（只存数组）或任何没记录到的 id，都该走默认值，而不是被当成「关」
      localStorage.setItem('dashboard-visible-charts', JSON.stringify({ modelDistribution: false }))

      const wrapper = mountDashboard()
      await flushPromises()

      expect(wrapper.findComponent(RangeUsageSummary).exists()).toBe(true)
      expect(wrapper.findComponent(TokenCompositionBar).exists()).toBe(true)
      expect(wrapper.findComponent({ name: 'ModelDistributionChart' }).exists()).toBe(false)
    })

    it('gives the only visible half-width chart the whole row, and shares the row once there are two', async () => {
      localStorage.setItem('dashboard-visible-charts', JSON.stringify({ tokenComposition: false }))

      const wrapper = mountDashboard()
      await flushPromises()

      expect(
        wrapper.findComponent({ name: 'ModelDistributionChart' }).classes()
      ).toContain('lg:col-span-2')

      await openChartSettings(wrapper)
      await toggleChart(wrapper, 'tokenComposition')

      expect(
        wrapper.findComponent({ name: 'ModelDistributionChart' }).classes()
      ).not.toContain('lg:col-span-2')
    })

    it('gives the last half-width chart the whole row when an odd number of them are visible', async () => {
      const wrapper = mountDashboard()
      await flushPromises()

      // 默认两张半栏，各占一半
      expect(
        wrapper.findComponent({ name: 'ModelDistributionChart' }).classes()
      ).not.toContain('lg:col-span-2')

      await openChartSettings(wrapper)
      await toggleChart(wrapper, 'tokenUsageTrend')

      // 三张半栏，最后一张撑满整行，避免右边空半格
      expect(wrapper.findComponent(TokenUsageStackedBar).classes()).toContain('lg:col-span-2')
    })

    it('renders the recent usage chart full width', async () => {
      const wrapper = mountDashboard()
      await flushPromises()
      await openChartSettings(wrapper)
      await toggleChart(wrapper, 'recentUsage')

      expect(wrapper.find('[data-testid="recent-usage-chart"]').classes()).toContain('lg:col-span-2')
    })
  })
})
