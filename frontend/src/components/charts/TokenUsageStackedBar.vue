<!-- FORK: 二开文件——仪表盘的「Token 使用趋势」改用堆叠柱状图。
     上游的 TokenUsageTrend.vue 是四条填充折线加一根右轴百分比线，两条轴互相压着看不清；
     这里只保留一条 token 轴，四段堆叠。缓存率不再画在这里——它已经在「区间用量概览」和
     「Token 构成占比」两张卡片里了。共享的 TokenUsageTrend.vue 保持原样，只有仪表盘用它。 -->
<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.dashboard.tokenUsageTrend') }}
    </h3>
    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="trendData.length > 0 && chartData" class="h-48">
      <Bar :data="chartData" :options="barOptions" />
    </div>
    <div
      v-else
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Title,
  Tooltip,
  Legend
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { TrendDataPoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, BarElement, Title, Tooltip, Legend)

const { t } = useI18n()

const props = defineProps<{
  trendData: TrendDataPoint[]
  loading?: boolean
}>()

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  input: '#3b82f6',
  output: '#10b981',
  cacheCreation: '#f59e0b',
  cacheRead: '#06b6d4'
}))

// 颜色沿用上游折线图那一套，四段在同一根柱子里堆起来
const chartData = computed(() => {
  if (!props.trendData?.length) return null

  return {
    labels: props.trendData.map((item) => item.date),
    datasets: [
      {
        label: 'Input',
        data: props.trendData.map((item) => item.input_tokens),
        backgroundColor: chartColors.value.input,
        stack: 'tokens'
      },
      {
        label: 'Output',
        data: props.trendData.map((item) => item.output_tokens),
        backgroundColor: chartColors.value.output,
        stack: 'tokens'
      },
      {
        label: 'Cache Creation',
        data: props.trendData.map((item) => item.cache_creation_tokens),
        backgroundColor: chartColors.value.cacheCreation,
        stack: 'tokens'
      },
      {
        label: 'Cache Read',
        data: props.trendData.map((item) => item.cache_read_tokens),
        backgroundColor: chartColors.value.cacheRead,
        stack: 'tokens'
      }
    ]
  }
})

const barOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.dataset.label}: ${formatTokens(Number(context.raw))}`,
        footer: (tooltipItems: any) => {
          const dataIndex = tooltipItems[0]?.dataIndex
          if (dataIndex === undefined || !props.trendData[dataIndex]) return ''
          const data = props.trendData[dataIndex]
          return `Actual: $${formatCost(data.actual_cost)} | Standard: $${formatCost(data.cost)}`
        }
      }
    }
  },
  scales: {
    x: {
      stacked: true,
      grid: { color: chartColors.value.grid },
      ticks: { color: chartColors.value.text, font: { size: 10 } }
    },
    y: {
      stacked: true,
      grid: { color: chartColors.value.grid },
      ticks: {
        color: chartColors.value.text,
        font: { size: 10 },
        callback: (value: string | number) => formatTokens(Number(value))
      }
    }
  }
}))

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const formatCost = (value: number): string => {
  if (value >= 1000) return (value / 1000).toFixed(2) + 'K'
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(4)
}
</script>
