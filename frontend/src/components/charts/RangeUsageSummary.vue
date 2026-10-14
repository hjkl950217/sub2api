<!-- FORK: 二开文件——仪表盘「区间用量概览」卡片。
     顶部那 8 张统计卡片是累计/今日口径，不随时间范围变；这张卡片补的是区间口径。
     数字样式照抄顶部统计卡片（彩色图标 + 小标题 + 大数字 + 小字副标题），装在同一个框里。 -->
<template>
  <div class="card p-4">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.dashboard.rangeSummary.title') }}
      </h3>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ rangeLabel }}</span>
    </div>

    <div v-if="loading && !hasData" class="flex h-16 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div v-for="item in items" :key="item.key" class="flex items-center gap-3">
        <div class="rounded-lg p-2" :class="item.iconBg">
          <Icon :name="item.icon" size="md" :class="item.iconColor" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">{{ item.value }}</p>
          <p v-if="item.sub" class="text-xs text-gray-500 dark:text-gray-400">{{ item.sub }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatNumber } from '@/utils/format'
import { cacheHitRate, formatRangeCost, formatRangeTokens, type RangeTotals } from './rangeTotals'

const { t } = useI18n()

const props = defineProps<{
  totals: RangeTotals
  /** 区间跨的天数，用来算日均；≤1 天时不显示日均（否则和总量是同一个数） */
  days: number
  /** 区间说明，如 `2026-10-08 ~ 2026-10-14` */
  rangeLabel: string
  loading?: boolean
}>()

const hasData = computed(() => props.totals.totalTokens > 0 || props.totals.requests > 0)
const showDaily = computed(() => props.days > 1)

interface SummaryItem {
  key: string
  label: string
  value: string
  sub: string
  icon: 'cube' | 'chart' | 'database' | 'clock'
  iconBg: string
  iconColor: string
}

const items = computed<SummaryItem[]>(() => {
  const dailyMultiplier = showDaily.value ? props.days : 0

  return [
    {
      key: 'totalTokens',
      label: t('admin.dashboard.rangeSummary.totalTokens'),
      value: formatRangeTokens(props.totals.totalTokens),
      sub: dailyMultiplier
        ? t('admin.dashboard.rangeSummary.dailyTokens', {
            value: formatRangeTokens(props.totals.totalTokens / dailyMultiplier)
          })
        : '',
      icon: 'cube',
      iconBg: 'bg-amber-100 dark:bg-amber-900/30',
      iconColor: 'text-amber-600 dark:text-amber-400'
    },
    {
      key: 'requests',
      label: t('admin.dashboard.rangeSummary.requests'),
      value: formatNumber(props.totals.requests),
      sub: dailyMultiplier
        ? t('admin.dashboard.rangeSummary.dailyRequests', {
            value: formatNumber(Math.round(props.totals.requests / dailyMultiplier))
          })
        : '',
      icon: 'chart',
      iconBg: 'bg-green-100 dark:bg-green-900/30',
      iconColor: 'text-green-600 dark:text-green-400'
    },
    {
      key: 'cacheRate',
      label: t('admin.dashboard.rangeSummary.cacheRate'),
      value: `${cacheHitRate(props.totals).toFixed(1)}%`,
      sub: t('admin.dashboard.rangeSummary.cacheSaved', {
        value: formatRangeTokens(props.totals.cacheReadTokens)
      }),
      icon: 'database',
      iconBg: 'bg-indigo-100 dark:bg-indigo-900/30',
      iconColor: 'text-indigo-600 dark:text-indigo-400'
    },
    {
      key: 'actualCost',
      label: t('admin.dashboard.rangeSummary.actualCost'),
      value: formatRangeCost(props.totals.actualCost),
      sub: t('admin.dashboard.rangeSummary.standardCost', {
        value: formatRangeCost(props.totals.cost)
      }),
      icon: 'clock',
      iconBg: 'bg-rose-100 dark:bg-rose-900/30',
      iconColor: 'text-rose-600 dark:text-rose-400'
    }
  ]
})
</script>
