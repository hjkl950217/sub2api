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
                <span
                  v-for="result in row.protocols"
                  :key="result.protocol"
                  class="rounded px-1.5 py-0.5 text-[11px] font-medium"
                  :class="result.success
                    ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
                    : 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400'"
                >
                  {{ shortProtocolLabel(result.protocol) }}
                </span>
                <button
                  v-if="row.excerpt"
                  type="button"
                  class="text-xs text-primary-600 hover:text-primary-700 dark:text-primary-300"
                  @click="row.expanded = !row.expanded"
                >
                  {{ row.expanded ? t('admin.accounts.batchTest.collapse') : t('admin.accounts.batchTest.expand') }}
                </button>
              </template>
              <span v-else class="text-xs text-gray-400">{{ row.reason }}</span>

              <span class="ml-auto shrink-0 text-xs font-medium" :class="statusClass(row)">
                {{ statusLabel(row) }}
              </span>
            </div>
            <div
              v-if="row.expanded && row.excerpt"
              class="max-h-[160px] overflow-y-auto border-t border-gray-100 bg-gray-900 px-3 py-2 font-mono text-xs text-green-300 dark:border-dark-600"
            >
              {{ row.excerpt }}
            </div>
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <!-- FORK: 批量把各账号测通的协议写回各自的 api_protocols -->
        <button
          v-if="probedRows.length > 0"
          type="button"
          data-testid="batch-update-protocols-button"
          :disabled="running || savingProtocols"
          :title="t('admin.accounts.syncProtocolsHint')"
          :class="[
            'mr-auto flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            running || savingProtocols
              ? 'cursor-not-allowed bg-indigo-300 text-white'
              : 'bg-indigo-500 text-white hover:bg-indigo-600'
          ]"
          @click="updateProtocols"
        >
          <Icon name="refresh" size="sm" :stroke-width="2" :class="savingProtocols ? 'animate-spin' : ''" />
          <span>{{ t('admin.accounts.batchTest.updateProtocols') }}</span>
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
  success: boolean
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
  excerpt: string
  expanded: boolean
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

// FORK: 有测通协议的行才允许写回
const probedRows = computed(() => rows.value.filter((row) => row.protocols.some((r) => r.success)))

const canStart = computed(
  () => !running.value && !loadingPlan.value && rows.value.some((row) => row.eligible) &&
    groups.value.some((group) => group.common_models.length > 0)
)

const shortProtocolLabel = (protocol: string) => {
  if (protocol === 'anthropic') return 'anthropic'
  if (protocol === 'responses') return 'responses'
  return 'chat'
}

const statusLabel = (row: BatchRow) => {
  switch (row.status) {
    case 'running':
      return t('admin.accounts.batchTest.testing')
    case 'success':
      return t('admin.accounts.batchTest.passed')
    case 'error':
      return row.error || t('admin.accounts.batchTest.failed')
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
          protocols: [],
          excerpt: '',
          expanded: false
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
    row.protocols = []
    row.excerpt = ''
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
}) => {
  if (typeof event.account_id !== 'number') return
  const row = rowsById.value.get(event.account_id)
  if (!row) return

  switch (event.type) {
    case 'protocol_result':
      if (event.protocol) {
        row.protocols.push({ protocol: event.protocol, success: event.protocol_ok === true })
      }
      if (event.text) {
        row.excerpt += `[${shortProtocolLabel(event.protocol || '')}]${event.text}\n`
      }
      break
    case 'content':
      if (event.text) row.excerpt += event.text
      break
    case 'error':
      row.status = 'error'
      row.error = event.error || ''
      break
    // 每个账号一定以 batch_test_complete 收尾，用它决定行状态
    case 'batch_test_complete':
      row.status = event.success === true ? 'success' : 'error'
      if (event.success !== true && !row.error) {
        row.error = event.error || t('admin.accounts.batchTest.failed')
      }
      break
  }
}

// FORK: 批量更新支持协议——按账号把各自测通的协议写回，复用单账号的写回接口
const updateProtocols = async () => {
  if (savingProtocols.value || running.value) return
  const targets = probedRows.value
  if (!targets.length) {
    appStore.showError(t('admin.accounts.batchTest.updateProtocolsNone'))
    return
  }
  savingProtocols.value = true
  let success = 0
  let failed = 0
  for (const row of targets) {
    const passed = row.protocols.filter((result) => result.success).map((result) => result.protocol)
    try {
      await adminAPI.accounts.updateProbedProtocols(row.id, passed)
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
