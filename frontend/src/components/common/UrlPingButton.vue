<!-- FORK: 账号表单 URL 输入框旁的连通性测试按钮（二开，可复用）。 -->
<template>
  <button
    type="button"
    class="inline-flex h-9 min-w-[68px] shrink-0 items-center justify-center gap-1 rounded-lg border px-2 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-60"
    :class="stateClass"
    :disabled="disabled || testing || !url.trim()"
    :title="tooltip"
    @click="run"
  >
    <svg v-if="testing" class="h-3.5 w-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
      <path
        class="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
    <Icon v-else name="play" size="xs" />
    <span>{{ label }}</span>
  </button>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { PingUrlResult } from '@/api/admin/accounts'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(
  defineProps<{
    url: string
    proxyId?: number | null
    disabled?: boolean
  }>(),
  { proxyId: null, disabled: false }
)

const { t } = useI18n()
const testing = ref(false)
const result = ref<PingUrlResult | null>(null)

// 地址改了，上次的结果就不再对应当前输入。
watch(
  () => props.url,
  () => {
    result.value = null
  }
)

const label = computed(() => {
  if (testing.value) return t('admin.accounts.pingTesting')
  if (!result.value) return t('admin.accounts.pingUrl')
  return result.value.success ? `${result.value.latency_ms}ms` : t('admin.accounts.pingFailed')
})

const tooltip = computed(() => {
  if (!result.value) return ''
  if (result.value.success) {
    const code = result.value.status_code ? `HTTP ${result.value.status_code} · ` : ''
    return `${code}${result.value.latency_ms}ms`
  }
  return result.value.message || ''
})

const stateClass = computed(() => {
  if (!result.value) {
    return 'border-gray-300 text-gray-600 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-300 dark:hover:text-primary-400'
  }
  return result.value.success
    ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-400'
    : 'border-red-300 bg-red-50 text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400'
})

const run = async () => {
  const target = props.url.trim()
  if (!target || testing.value) return
  testing.value = true
  result.value = null
  try {
    result.value = await adminAPI.accounts.pingUrl({
      url: target,
      proxy_id: props.proxyId && props.proxyId > 0 ? props.proxyId : null
    })
  } catch (error: any) {
    result.value = {
      success: false,
      latency_ms: 0,
      message: error?.response?.data?.message || String(error)
    }
  } finally {
    testing.value = false
  }
}
</script>
