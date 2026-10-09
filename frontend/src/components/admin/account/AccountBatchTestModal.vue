<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.batchTest.title')"
    width="wide"
    @close="handleClose"
  >
    <div class="space-y-4">
      <!-- 摘要：选中了多少个、其中多少个能测 -->
      <div class="rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm dark:border-dark-500 dark:bg-dark-700">
        <span class="text-gray-700 dark:text-gray-200">{{ summaryText }}</span>
      </div>

      <div v-if="loadingPlan" class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}...
      </div>

      <div v-else-if="!groups.length" class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.batchTest.noEligible') }}
      </div>

      <template v-else>
        <div class="space-y-1.5">
          <TextArea
            v-model="testPrompt"
            :label="t('admin.accounts.batchTest.promptLabel')"
            :disabled="running"
            rows="2"
          />
        </div>

        <!-- 每个平台一组：组头选模型，组内逐账号一行状态 -->
        <div
          v-for="group in groups"
          :key="group.platform"
          class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-500"
        >
          <div class="flex flex-wrap items-center gap-3 border-b border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-500 dark:bg-dark-700">
            <span class="font-medium text-gray-800 dark:text-gray-100">{{ group.platform }}</span>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.batchTest.accountCount', { count: group.accounts.length }) }}
            </span>
            <span class="ml-auto flex items-center gap-2">
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.batchTest.commonModels', { count: group.common_models.length }) }}
              </span>
              <Select
                v-model="selectedModels[group.platform]"
                :options="modelOptions(group)"
                :disabled="running || !group.common_models.length"
                value-key="id"
                label-key="display_name"
                :placeholder="t('admin.accounts.selectTestModel')"
              />
            </span>
          </div>

          <div
            v-if="!group.common_models.length"
            class="border-b border-gray-100 px-3 py-2 text-xs text-amber-600 dark:border-dark-600 dark:text-amber-400"
          >
            {{ t('admin.accounts.batchTest.noCommonModel') }}
          </div>

          <div
            v-for="row in rowsForPlatform(group.platform)"
            :key="row.id"
            :data-testid="`batch-row-${row.id}`"
            class="border-b border-gray-100 last:border-b-0 dark:border-dark-600"
          >
            <div class="flex items-center gap-2 px-3 py-2 text-sm">
              <span class="font-mono text-xs text-gray-400">#{{ row.id }}</span>
              <span class="truncate text-gray-800 dark:text-gray-200">{{ row.name }}</span>

              <template v-if="row.eligible">
                <!-- FORK: 协议标签四态——未测灰、测试中黄、通过绿、失败红，打开弹窗就铺好占位 -->
                <span
                  v-for="result in row.protocols"
                  :key="result.protocol"
                  :data-testid="`batch-protocol-${row.id}-${result.protocol}`"
                  class="rounded px-1.5 py-0.5 text-[11px] font-medium"
                  :class="protocolClass(result)"
                >
                  {{ shortProtocolLabel(result.protocol) }}
                </span>
              </template>
              <span v-else class="text-xs text-gray-400">{{ row.reason }}</span>

              <span class="ml-auto flex shrink-0 items-center gap-2">
                <!-- FORK: 逐行调度开关，不用回账号列表就能开关调度 -->
                <Toggle
                  :data-testid="`batch-schedulable-${row.id}`"
                  :model-value="row.schedulable"
                  :title="t('admin.accounts.schedulable')"
                  @update:model-value="setSchedulable(row, $event)"
                />
                <span class="text-xs font-medium" :class="statusClass(row)">
                  {{ statusLabel(row) }}
                </span>
              </span>
            </div>

            <!-- FORK-ANCHOR: batch-test-protocol-lines (二开：一个账号的每个协议各占一行，不再挤在同一行)
                 失败信息只在这里显示一次：失败协议优先显示完整报错，通过协议显示返回正文。 -->
            <div
              v-if="row.eligible && row.protocols.length > 0"
              class="space-y-1 border-t border-gray-100 bg-gray-50 px-3 py-2 font-mono text-xs dark:border-dark-600 dark:bg-gray-900"
            >
              <div
                v-for="result in row.protocols"
                :key="`line-${result.protocol}`"
                class="flex items-center gap-1 rounded px-1 py-0.5"
                :class="result.success === false ? 'bg-red-500/10' : ''"
                :data-testid="`batch-protocol-line-${row.id}-${result.protocol}`"
              >
                <span
                  class="shrink-0 rounded px-1.5 py-0.5 text-[11px] font-medium"
                  :class="protocolClass(result)"
                >
                  {{ shortProtocolLabel(result.protocol) }}
                </span>
                <span class="shrink-0 text-[11px]" :class="protocolTextClass(result)">
                  {{ protocolStatusLabel(result) }}
                </span>
                <!-- FORK-ANCHOR: batch-test-timing-display (二开：状态与正文之间显示「首字/完整」耗时) -->
                <span
                  v-if="timingLabel(result)"
                  class="shrink-0 text-[11px] text-gray-400"
                  :data-testid="`batch-protocol-timing-${row.id}-${result.protocol}`"
                >
                  {{ timingLabel(result) }}
                </span>
                <!-- FORK-ANCHOR: batch-test-body-neutral-color (二开：正文用中性色，颜色只留给协议状态，绿色会被当成「返回值」)
                     成功/失败只靠左侧状态标签和失败行的红底区分，正文本身不着色。 -->
                <span
                  class="min-w-0 flex-1 whitespace-pre-wrap break-all text-gray-700 dark:text-gray-300"
                  :class="lineClampClass(result)"
                >
                  <template v-if="result.success === null && result.probing">{{ t('admin.accounts.batchTest.testing') }}</template>
                  <template v-else-if="result.success === null">{{ t('admin.accounts.batchTest.pending') }}</template>
                  <template v-else-if="result.success === false && result.error">{{ result.error }}</template>
                  <template v-else-if="result.body">{{ result.body }}</template>
                  <template v-else>{{ dashPlaceholder }}</template>
                </span>
                <!-- FORK-ANCHOR: batch-test-protocol-toggle (二开：展开/收起按「单个账号的单个协议」，
                     长正文默认折叠 2 行，只展开这一条协议的结果) -->
                <button
                  v-if="protocolLineTruncated(result)"
                  type="button"
                  class="shrink-0 text-[11px] text-primary-600 hover:text-primary-700 dark:text-primary-300"
                  :data-testid="`batch-protocol-toggle-${row.id}-${result.protocol}`"
                  @click="result.expanded = !result.expanded"
                >
                  {{ result.expanded ? t('admin.accounts.batchTest.collapse') : t('admin.accounts.batchTest.expand') }}
                </button>
              </div>
            </div>

            <!-- FORK-ANCHOR: batch-test-row-failure-line (二开：行级失败原因，展开与否都直接可见)
                 逐协议报错之外还有收尾错误（超时、无完成事件等），它们不属于任何协议，
                 这里统一显示，收起正文时也不会漏掉失败原因。 -->
            <div
              v-if="row.status === 'error' && row.rowFailure"
              class="whitespace-pre-wrap break-all border-t border-red-100 bg-red-50 px-3 py-1.5 font-mono text-[11px] text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-400"
              :data-testid="`batch-failure-${row.id}`"
            >
              {{ row.rowFailure }}
            </div>
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <!-- FORK: 批量把各账号测通的协议写回各自的 api_protocols。
             按钮常显，没有可写回的行时置灰；测试进行中也能点，只写已经跑完的账号。 -->
        <button
          type="button"
          data-testid="batch-update-protocols-button"
          :disabled="savingProtocols || probedRows.length === 0"
          :title="t('admin.accounts.syncProtocolsHint')"
          :class="[
            'mr-auto flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            savingProtocols
              ? 'cursor-not-allowed bg-indigo-300 text-white'
              : probedRows.length === 0
                ? 'cursor-not-allowed bg-gray-100 text-gray-400 dark:bg-dark-600 dark:text-gray-500'
                : 'bg-indigo-500 text-white hover:bg-indigo-600'
          ]"
          @click="updateProtocols"
        >
          <Icon name="refresh" size="sm" :stroke-width="2" :class="savingProtocols ? 'animate-spin' : ''" />
          <span>{{ t('admin.accounts.batchTest.updateProtocolsCount', { count: probedRows.length }) }}</span>
        </button>
        <button
          type="button"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          @click="handleClose"
        >
          {{ t('common.close') }}
        </button>
        <button
          type="button"
          data-testid="batch-test-start-button"
          :disabled="!canStart"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            canStart ? 'bg-primary-500 text-white hover:bg-primary-600' : 'cursor-not-allowed bg-primary-400 text-white'
          ]"
          @click="startTest"
        >
          <Icon :name="running ? 'refresh' : 'play'" size="sm" :stroke-width="2" :class="running ? 'animate-spin' : ''" />
          <span>{{ running ? t('admin.accounts.batchTest.testing') : t('admin.accounts.batchTest.start') }}</span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import Toggle from '@/components/common/Toggle.vue'
import { Icon } from '@/components/icons'
import { adminAPI } from '@/api/admin'
import { buildApiUrl } from '@/api/client'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'
import { useAppStore } from '@/stores/app'
import type { BatchTestAccountGroup } from '@/api/admin/accounts'

const { t } = useI18n()
const appStore = useAppStore()

interface ProtocolResult {
  protocol: string
  // null = 还没测到（灰色占位），true/false 才是探测结论
  success: boolean | null
  // FORK-ANCHOR: batch-test-protocol-probing-field (二开：该协议正在探测中，标签显示「测试中」中间态)
  probing: boolean
  // FORK-ANCHOR: batch-test-protocol-error-field (二开：该协议失败时后端给回的完整报错)
  error: string
  // FORK-ANCHOR: batch-test-protocol-body-field (二开：该协议返回的正文，失败时也保留)
  body: string
  // FORK-ANCHOR: batch-test-protocol-expanded-field (二开：展开/收起是按「单个账号的单个协议」，
  // 长正文默认折叠 2 行，各自展开看全文)
  expanded: boolean
  // FORK-ANCHOR: batch-test-protocol-timing-field (二开：首字响应 / 完整响应耗时，单位毫秒)
  firstByteMs: number | null
  totalMs: number | null
}

interface BatchRow {
  id: number
  name: string
  platform: string
  eligible: boolean
  reason: string
  status: 'idle' | 'running' | 'success' | 'error'
  error: string
  protocols: ProtocolResult[]
  // FORK-ANCHOR: batch-test-row-failure-field (二开：不属于任何协议的失败原因，如超时、无完成事件)
  // 逐协议报错挂在各自的 ProtocolResult.error 上，不重复进这里。
  rowFailure: string
  schedulable: boolean
}

const props = defineProps<{
  show: boolean
  accountIds: number[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated'): void
}>()

const groups = ref<BatchTestAccountGroup[]>([])
const rows = ref<BatchRow[]>([])
const selectedModels = ref<Record<string, string>>({})
const testPrompt = ref('')
const loadingPlan = ref(false)
const running = ref(false)
const savingProtocols = ref(false)
const togglingSchedulable = ref<number | null>(null)
let abortController: AbortController | null = null

const rowsById = computed(() => {
  const map = new Map<number, BatchRow>()
  rows.value.forEach((row) => map.set(row.id, row))
  return map
})

const rowsForPlatform = (platform: string) => rows.value.filter((row) => row.platform === platform)

const modelOptions = (group: BatchTestAccountGroup) =>
  group.common_models.map((model) => ({ id: model, display_name: model }))

const summaryText = computed(() =>
  t('admin.accounts.batchTest.selectedSummary', {
    count: props.accountIds.length,
    eligible: rows.value.filter((row) => row.eligible).length
  })
)

// FORK: 可写回的行 = 已经跑完（通过/失败）且至少有一个协议测通。
// 还在跑的行不能进：协议没探完，按当前结果写回会把后面才通的那几个协议漏掉。
const probedRows = computed(() =>
  rows.value.filter(
    (row) => (row.status === 'success' || row.status === 'error') && row.protocols.some((r) => r.success)
  )
)

const canStart = computed(
  () => !running.value && !loadingPlan.value && rows.value.some((row) => row.eligible) &&
    groups.value.some((group) => group.common_models.length > 0)
)

// FORK-ANCHOR: batch-test-protocol-four-state (二开：协议标签四态——未测灰、测试中黄、通过绿、失败红)
const protocolClass = (result: ProtocolResult) => {
  if (result.success === true) return 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
  if (result.success === false) return 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400'
  if (protocolProbing(result)) return 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400'
  return 'bg-gray-100 text-gray-400 dark:bg-dark-600 dark:text-gray-500'
}

// FORK-ANCHOR: batch-test-protocol-probing-class (二开：探测中即「测试中」中间态)
const protocolProbing = (result: ProtocolResult) => result.probing && result.success === null

// FORK-ANCHOR: batch-test-body-theme-aware (二开：正文区跟随主题，浅色下不再是一块深色终端)
const protocolTextClass = (result: ProtocolResult) => {
  if (result.success === true) return 'text-green-600 dark:text-green-300'
  if (result.success === false) return 'text-red-600 dark:text-red-300'
  if (protocolProbing(result)) return 'text-amber-600 dark:text-amber-300'
  return 'text-gray-500'
}

const protocolStatusLabel = (result: ProtocolResult) => {
  if (result.success === true) return t('admin.accounts.protocolProbePassed')
  if (result.success === false) return t('admin.accounts.protocolProbeFailed')
  if (protocolProbing(result)) return t('admin.accounts.batchTest.testing')
  return t('admin.accounts.batchTest.pending')
}

const shortProtocolLabel = (protocol: string) => {
  if (protocol === 'anthropic') return 'anthropic'
  if (protocol === 'responses') return 'responses'
  return 'chat'
}

// 占位符：协议测通但没有正文时用它，避免该行看起来是空的
const dashPlaceholder = '—'

// FORK-ANCHOR: batch-test-protocol-line-clamp (二开：逐协议正文默认折叠 2 行，展开只针对这一条协议)
const lineClampClass = (result: ProtocolResult) => (result.expanded ? '' : 'line-clamp-2')

// 该条协议正文/报错较长时给「展开/收起」；是否已展开不影响按钮是否出现，
// 否则展开一次就再也收不回去（FORK-ANCHOR: batch-test-toggle-always-visible）。
const protocolLineTruncated = (result: ProtocolResult) =>
  (result.error || result.body || '').length > 120

// FORK-ANCHOR: batch-test-protocol-timing (二开：状态与正文之间显示「首字/完整」耗时，如 0.1s/1.2s)
const formatSeconds = (ms: number) => `${(ms / 1000).toFixed(1)}s`
const timingLabel = (result: ProtocolResult) => {
  if (result.totalMs == null || result.totalMs <= 0) return ''
  if (result.firstByteMs == null || result.firstByteMs <= 0) return formatSeconds(result.totalMs)
  return `${formatSeconds(result.firstByteMs)}/${formatSeconds(result.totalMs)}`
}

// FORK-ANCHOR: batch-test-failed-with-body (二开：失败协议的报错去重辅助)
// 报错现在只在逐协议行显示，这个辅助保留给行级失败判定用：失败协议的报错集合。
const failedProtocolErrors = (row: BatchRow) =>
  row.protocols
    .filter((result) => result.success === false && result.error)
    .map((result) => result.error)

const statusLabel = (row: BatchRow) => {
  switch (row.status) {
    case 'running':
      return t('admin.accounts.batchTest.testing')
    case 'success':
      return t('admin.accounts.batchTest.passed')
    case 'error':
      // FORK-ANCHOR: batch-test-header-short-status (二开：行头只显示短状态，完整报错放在正文区与失败行，
      // 否则调度开关旁边会糊上一整段上游 HTML 报错)
      return t('admin.accounts.batchTest.failed')
    default:
      return t('admin.accounts.batchTest.pending')
  }
}

const statusClass = (row: BatchRow) => {
  if (row.status === 'success') return 'text-green-600 dark:text-green-400'
  if (row.status === 'error') return 'text-red-600 dark:text-red-400'
  if (row.status === 'running') return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-400'
}

const loadPlan = async () => {
  abortStream()
  loadingPlan.value = true
  groups.value = []
  rows.value = []
  selectedModels.value = {}
  testPrompt.value = t('admin.accounts.textPromptDefault')
  try {
    const plan = await adminAPI.accounts.getBatchTestPlan(props.accountIds)
    const nextModels: Record<string, string> = {}
    const nextRows: BatchRow[] = []
    plan.groups.forEach((group) => {
      nextModels[group.platform] = group.common_models[0] || ''
      group.accounts.forEach((account) => {
        nextRows.push({
          id: account.id,
          name: account.name,
          platform: group.platform,
          eligible: account.eligible,
          reason: account.reason || '',
          status: 'idle',
          error: '',
          // 打开就按后端给的协议清单铺占位标签，测试推进时逐个变色
          protocols: (account.protocols || []).map((protocol) => ({ protocol, success: null, probing: false, error: '', body: '', expanded: false, firstByteMs: null, totalMs: null })),
          rowFailure: '',
          schedulable: account.schedulable
        })
      })
    })
    groups.value = plan.groups
    selectedModels.value = nextModels
    rows.value = nextRows
  } catch (error) {
    appStore.showError(error instanceof Error ? error.message : t('common.unknownError'))
  } finally {
    loadingPlan.value = false
  }
}

const abortStream = () => {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
}

const handleClose = () => {
  abortStream()
  emit('close')
}

watch(
  () => props.show,
  (visible) => {
    if (visible && props.accountIds.length > 0) {
      void loadPlan()
    } else if (!visible) {
      abortStream()
    }
  }
)

const buildTargets = () => {
  const targets: Array<{ account_id: number; model_id: string; prompt: string }> = []
  groups.value.forEach((group) => {
    const modelID = selectedModels.value[group.platform]
    if (!modelID) return
    group.accounts.forEach((account) => {
      if (!account.eligible) return
      targets.push({ account_id: account.id, model_id: modelID, prompt: testPrompt.value.trim() })
    })
  })
  return targets
}

const startTest = async () => {
  const targets = buildTargets()
  if (!targets.length || running.value) return

  rows.value.forEach((row) => {
    row.status = row.eligible ? 'running' : 'idle'
    row.error = ''
    // 占位标签留着，只把颜色退回未测
    row.protocols.forEach((result) => {
      result.success = null
      result.error = ''
      result.body = ''
      result.expanded = false
      result.firstByteMs = null
      result.totalMs = null
    })
    row.rowFailure = ''
  })
  running.value = true
  abortStream()
  abortController = new AbortController()

  try {
    const response = await fetch(buildApiUrl('/admin/accounts/batch-test'), {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
        'Content-Type': 'application/json',
        [ADMIN_UI_REQUEST_HEADER]: '1'
      },
      body: JSON.stringify({ targets }),
      signal: abortController.signal
    })
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }
    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error(t('admin.accounts.grok.noResponseBody'))
    }

    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        const payload = line.slice(6).trim()
        if (!payload) continue
        try {
          handleEvent(JSON.parse(payload))
        } catch (error) {
          console.error('Failed to parse batch test event:', error)
        }
      }
    }
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === 'AbortError') return
    appStore.showError(error instanceof Error ? error.message : t('common.unknownError'))
    rows.value.forEach((row) => {
      if (row.status === 'running') row.status = 'error'
    })
  } finally {
    running.value = false
  }
}

const handleEvent = (event: {
  type: string
  account_id?: number
  text?: string
  error?: string
  success?: boolean
  protocol?: string
  protocol_ok?: boolean
  // FORK-ANCHOR: batch-test-protocol-timing-event-field (二开：逐协议耗时，毫秒)
  first_byte_ms?: number
  total_ms?: number
}) => {
  if (typeof event.account_id !== 'number') return
  const row = rowsById.value.get(event.account_id)
  if (!row) return

  switch (event.type) {
    // FORK-ANCHOR: batch-test-protocol-probing-event (二开：探测开始时标签变「测试中」中间态)
    // 占位标签在打开弹窗时就铺好了，探测开始先把那一个标成进行中，用户能看出测到哪了。
    case 'protocol_probe':
      if (event.protocol) {
        const probing = row.protocols.find((result) => result.protocol === event.protocol)
        if (probing) {
          probing.probing = true
        }
      }
      break
    case 'protocol_result':
      if (event.protocol) {
        // 占位标签在打开弹窗时就建好了，这里只改颜色；清单外的协议才追加（兜底）。
        const probed = row.protocols.find((result) => result.protocol === event.protocol)
        const target =
          probed ||
          (() => {
            const created = { protocol: event.protocol as string, success: null, probing: false, error: '', body: '', expanded: false, firstByteMs: null, totalMs: null }
            row.protocols.push(created)
            return created
          })()
        target.success = event.protocol_ok === true
        target.probing = false
        // FORK-ANCHOR: batch-test-protocol-error-capture (二开：失败协议连完整报错一起记下)
        target.error = event.error || ''
        // FORK-ANCHOR: batch-test-protocol-body-capture (二开：协议返回的正文单独存，逐协议行显示)
        target.body = event.text || ''
        // FORK-ANCHOR: batch-test-protocol-timing-capture (二开：记下首字与完整耗时)
        target.firstByteMs = typeof event.first_byte_ms === 'number' ? event.first_byte_ms : null
        target.totalMs = typeof event.total_ms === 'number' ? event.total_ms : null
      }
      break
    case 'error':
      row.status = 'error'
      row.error = event.error || ''
      // FORK-ANCHOR: batch-test-error-append (二开：整账号级错误也进失败原因，收起正文时仍可见)
      if (event.error) row.rowFailure = appendFailure(row.rowFailure, event.error)
      break
    // 每个账号一定以 batch_test_complete 收尾，用它决定行状态
    case 'batch_test_complete':
      row.status = event.success === true ? 'success' : 'error'
      if (event.success !== true) {
        // FORK-ANCHOR: batch-test-complete-failure-capture (二开：收尾事件只收非协议类失败)
        // 后端收尾可能把各失败协议的报错 join 汇总成一段文本，逐协议行已经各自显示过了；
        // 这里逐段拆开，凡是能在失败协议报错里找到的段落都跳过，只把「非协议类」失败
        // （超时、未返回完成事件等）挂到行级失败原因，避免同一段文本显示多遍。
        const knownErrors = failedProtocolErrors(row)
        const segments = (event.error || '').split('\n').map((s) => s.trim()).filter(Boolean)
        const novel = segments.filter(
          (segment) => !knownErrors.some((known) => known === segment || known.includes(segment) || segment.includes(known))
        )
        for (const segment of novel) {
          row.rowFailure = appendFailure(row.rowFailure, segment)
        }
        if (event.error && !novel.length && !event.error.includes('\n')) {
          // 整段都是已知协议报错的拼接：不重复挂；但行级 error 仍要有内容供状态列显示
          row.error = row.error || segments[0] || ''
        }
        const hasProtocolFailure = row.protocols.some((result) => result.success === false)
        if (!row.rowFailure && !row.error && !hasProtocolFailure) {
          row.rowFailure = t('admin.accounts.batchTest.failed')
        }
      }
      // FORK-ANCHOR: batch-test-finish-gate (二开：所有可测账号都出结论即视为这批结束，
      // 不再干等 SSE 连接自己关闭——上游偶发不关连接会让按钮一直卡在「测试中」)
      if (running.value) {
        const eligible = rows.value.filter((item) => item.eligible)
        if (eligible.length > 0 && eligible.every((item) => item.status === 'success' || item.status === 'error')) {
          running.value = false
          abortStream()
        }
      }
      break
  }
}

// FORK-ANCHOR: batch-test-append-failure (二开：多段失败信息按行拼接，不覆盖前面的)
const appendFailure = (current: string, next: string) => {
  if (!current) return next
  if (current.includes(next)) return current
  return `${current}\n${next}`
}

// FORK: 逐行开关调度，复用账号列表用的同一个接口
const setSchedulable = async (row: BatchRow, next: boolean) => {
  if (togglingSchedulable.value !== null) return
  const previous = row.schedulable
  row.schedulable = next
  togglingSchedulable.value = row.id
  try {
    const updated = await adminAPI.accounts.setSchedulable(row.id, next)
    row.schedulable = updated?.schedulable ?? next
    emit('updated')
  } catch (error) {
    row.schedulable = previous
    console.error('Failed to toggle schedulable:', error)
    appStore.showError(t('admin.accounts.failedToToggleSchedulable'))
  } finally {
    togglingSchedulable.value = null
  }
}

// FORK: 批量更新支持协议——按账号把各自测通的协议写回，复用单账号的写回接口。
// 测试进行中也允许调用，只写已经跑完的行。
const updateProtocols = async () => {
  if (savingProtocols.value) return
  // 先把要写的内容快照下来：运行中还有账号在收事件，边写边取会拿到半截结果。
  const targets = probedRows.value.map((row) => ({
    id: row.id,
    passed: row.protocols.filter((result) => result.success).map((result) => result.protocol)
  }))
  if (!targets.length) {
    appStore.showError(t('admin.accounts.batchTest.updateProtocolsNone'))
    return
  }
  savingProtocols.value = true
  let success = 0
  let failed = 0
  for (const target of targets) {
    try {
      await adminAPI.accounts.updateProbedProtocols(target.id, target.passed)
      success++
    } catch (error) {
      console.error('Failed to update probed protocols:', error)
      failed++
    }
  }
  savingProtocols.value = false
  if (failed > 0) {
    appStore.showError(t('admin.accounts.batchTest.updateProtocolsPartial', { success, failed }))
  } else {
    appStore.showSuccess(t('admin.accounts.batchTest.updateProtocolsDone', { count: success }))
  }
  emit('updated')
}
</script>
