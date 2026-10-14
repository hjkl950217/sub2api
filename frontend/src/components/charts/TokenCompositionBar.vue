<!-- FORK: 二开文件——仪表盘「Token 构成占比」卡片。
     把区间内的 token 拆成输入/输出/缓存写/缓存读四段，比折线图更容易看出缓存吃掉了多少。
     四段之和等于 total_tokens（后端 usage_log_repo_trend.go 就是这么算的）。 -->
<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.dashboard.tokenComposition.title') }}
    </h3>

    <div v-if="loading && !hasData" class="flex h-24 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="!hasData"
      class="flex h-24 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
    <template v-else>
      <div class="flex h-3 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
        <div
          v-for="segment in segments"
          :key="segment.key"
          :data-composition-segment="segment.key"
          :class="segment.barClass"
          :style="{ width: segment.width }"
        />
      </div>

      <ul class="mt-4 divide-y divide-gray-200 dark:divide-dark-700">
        <li
          v-for="segment in segments"
          :key="segment.key"
          class="flex items-center gap-3 py-2 first:pt-0 last:pb-0"
        >
          <span class="h-2.5 w-2.5 flex-shrink-0 rounded-full" :class="segment.barClass" />
          <span class="min-w-0 flex-1 text-sm text-gray-700 dark:text-gray-300">
            {{ segment.label }}
          </span>
          <span class="text-sm text-gray-500 dark:text-gray-400">{{ segment.tokens }}</span>
          <span class="w-14 text-right text-sm font-medium text-gray-900 dark:text-white">
            {{ segment.percent }}
          </span>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatNumber } from '@/utils/format'
import { type RangeTotals } from './rangeTotals'

const { t } = useI18n()

const props = defineProps<{
  totals: RangeTotals
  loading?: boolean
}>()

const hasData = computed(() => props.totals.totalTokens > 0)

const segments = computed(() => {
  const total = props.totals.totalTokens

  const buckets = [
    {
      key: 'input',
      label: t('admin.dashboard.tokenComposition.input'),
      tokens: props.totals.inputTokens,
      barClass: 'bg-blue-500'
    },
    {
      key: 'output',
      label: t('admin.dashboard.tokenComposition.output'),
      tokens: props.totals.outputTokens,
      barClass: 'bg-emerald-500'
    },
    {
      key: 'cacheCreation',
      label: t('admin.dashboard.tokenComposition.cacheCreation'),
      tokens: props.totals.cacheCreationTokens,
      barClass: 'bg-amber-500'
    },
    {
      key: 'cacheRead',
      label: t('admin.dashboard.tokenComposition.cacheRead'),
      tokens: props.totals.cacheReadTokens,
      barClass: 'bg-cyan-500'
    }
  ]

  // 占比高的排前面；数值相等时保持清单顺序（Array.sort 是稳定排序）
  return buckets
    .slice()
    .sort((a, b) => b.tokens - a.tokens)
    .map(bucket => {
      const percent = total > 0 ? (bucket.tokens / total) * 100 : 0
      return {
        ...bucket,
        width: `${percent.toFixed(1)}%`,
        percent: `${percent.toFixed(1)}%`,
        tokens: formatNumber(bucket.tokens)
      }
    })
})
</script>
