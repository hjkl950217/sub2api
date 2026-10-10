<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-5 pb-8">
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-2">
          <input
            v-model="query"
            type="search"
            class="input flex-1 basis-64"
            :placeholder="t('groupModelAccounts.searchPlaceholder')"
          />
          <div class="tabs inline-flex">
            <button
              type="button"
              class="tab"
              :class="view === 'model' ? 'tab-active' : ''"
              @click="view = 'model'"
            >
              {{ t('groupModelAccounts.modelView') }}
            </button>
            <button
              type="button"
              class="tab"
              :class="view === 'group' ? 'tab-active' : ''"
              @click="view = 'group'"
            >
              {{ t('groupModelAccounts.groupView') }}
            </button>
          </div>
          <label class="inline-flex items-center gap-1.5 text-sm text-gray-500 dark:text-gray-400">
            <input
              v-model="onlyScheduled"
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600"
            />
            {{ t('groupModelAccounts.onlyScheduled') }}
          </label>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">
            <Icon name="refresh" size="sm" />
            {{ t('groupModelAccounts.refresh') }}
          </button>
          <span class="ml-auto text-xs text-gray-500 dark:text-gray-400">{{ metaText }}</span>
        </div>
      </div>

      <div v-if="error" class="card p-4 text-sm text-red-600 dark:text-red-400">{{ error }}</div>

      <div v-else-if="loading && !loadedAt" class="card p-4 text-sm text-gray-500 dark:text-gray-400">
        {{ t('groupModelAccounts.loading') }}
      </div>

      <template v-else-if="view === 'model'">
        <div v-if="!modelRows.length" class="card p-4 text-sm text-gray-500 dark:text-gray-400">
          {{ t('groupModelAccounts.emptyModel') }}
        </div>
        <div v-else class="card overflow-x-auto">
          <table class="table w-full">
            <thead>
              <tr>
                <th>{{ t('groupModelAccounts.colModel') }}</th>
                <th>{{ t('groupModelAccounts.colGroupAccount') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in modelRows" :key="row.model">
                <td class="whitespace-nowrap align-top font-semibold">{{ row.model }}</td>
                <td>
                  <div
                    v-for="entry in row.groups"
                    :key="entry.name"
                    class="flex flex-wrap items-center gap-1.5 py-0.5"
                  >
                    <span class="badge badge-primary">{{ entry.name }}</span>
                    <span class="text-xs text-gray-400 dark:text-dark-400">
                      {{ t('groupModelAccounts.count', { count: entry.accounts.length }) }}
                    </span>
                    <span
                      v-for="account in entry.accounts"
                      :key="account.id"
                      :class="['badge', account.schedulable ? 'badge-success' : 'badge-gray']"
                    >
                      {{ account.name }}
                    </span>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <template v-else>
        <div v-if="!groupRows.length" class="card p-4 text-sm text-gray-500 dark:text-gray-400">
          {{ t('groupModelAccounts.emptyGroup') }}
        </div>
        <div v-for="group in groupRows" :key="group.id" class="card overflow-hidden">
          <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <h2 class="flex flex-wrap items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
              {{ group.name }}
              <span class="badge badge-gray">{{ group.platform || t('groupModelAccounts.unlabeled') }}</span>
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('groupModelAccounts.groupMeta', { id: group.id }) }}
              ·
              {{ group.total > group.counted
                ? t('groupModelAccounts.accountSummaryExcluded', { counted: group.counted, total: group.total, excluded: group.total - group.counted })
                : t('groupModelAccounts.accountSummary', { counted: group.counted, total: group.total }) }}
            </p>
          </div>
          <div class="overflow-x-auto">
            <table v-if="group.models.length" class="table w-full">
              <thead>
                <tr>
                  <th>{{ t('groupModelAccounts.colModel') }}</th>
                  <th>{{ t('groupModelAccounts.colAccount') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="entry in group.models" :key="entry.model">
                  <td class="whitespace-nowrap align-top font-semibold">{{ entry.model }}</td>
                  <td>
                    <div class="flex flex-wrap items-center gap-1.5">
                      <span
                        v-for="account in entry.accounts"
                        :key="account.id"
                        :class="['badge', account.schedulable ? 'badge-success' : 'badge-gray']"
                      >
                        {{ account.name }}
                      </span>
                      <span class="text-xs text-gray-400 dark:text-dark-400">
                        {{ t('groupModelAccounts.count', { count: entry.accounts.length }) }}
                      </span>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
            <p v-else class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
              {{ t('groupModelAccounts.noGroupModels') }}
            </p>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
// FORK: 「分组模型账号」管理页。取代原先挂在 /custom/group-model-accounts 上的
// 运行时 DOM 注入面板（见 fork/CLAUDE.md 第 2.6c 节）：真路由页随镜像发布，
// 站点升级后不需要重建，也不受自定义页面 iframe 的 X-Frame-Options 限制。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { AccountListItem, AdminGroup } from '@/types'

interface AccountEntry {
  id: number
  name: string
  schedulable: boolean
  status: string
}

interface ModelGroupEntry {
  name: string
  accounts: AccountEntry[]
}

interface ModelRow {
  model: string
  groups: ModelGroupEntry[]
}

interface GroupModelEntry {
  model: string
  accounts: AccountEntry[]
}

interface GroupRow {
  id: number
  name: string
  platform: string
  total: number
  counted: number
  models: GroupModelEntry[]
}

const { t } = useI18n()

const groups = ref<AdminGroup[]>([])
const accountsByGroup = ref<Record<number, AccountListItem[]>>({})
const query = ref('')
const view = ref<'model' | 'group'>('model')
const onlyScheduled = ref(true)
const loading = ref(false)
const loadedAt = ref<Date | null>(null)
const error = ref('')

function isScheduled(account: AccountListItem): boolean {
  return account.status === 'active' && account.schedulable === true
}

function modelsOf(account: AccountListItem): string[] {
  const credentials = account.credentials ?? {}
  const mapping = (credentials.model_mapping ?? {}) as Record<string, unknown>
  return Object.keys(mapping)
    .filter((model) => model.trim() !== '')
    .sort((a, b) => a.localeCompare(b))
}

function sortAccounts(list: AccountEntry[]): AccountEntry[] {
  return [...list].sort(
    (left, right) =>
      Number(right.schedulable) - Number(left.schedulable) ||
      left.name.localeCompare(right.name, 'zh-CN') ||
      left.id - right.id,
  )
}

function toEntry(account: AccountListItem): AccountEntry {
  return {
    id: account.id,
    name: account.name || '#' + account.id,
    schedulable: isScheduled(account),
    status: account.status,
  }
}

const searchTerms = computed(() => query.value.toLocaleLowerCase().split(/\s+/).filter(Boolean))

function matched(values: string[]): boolean {
  const haystack = values.join(' ').toLocaleLowerCase()
  return searchTerms.value.every((term) => haystack.includes(term))
}

const modelRows = computed<ModelRow[]>(() => {
  const byModel = new Map<string, Map<string, AccountEntry[]>>()
  for (const group of groups.value) {
    for (const account of keptAccounts(group.id)) {
      for (const model of modelsOf(account)) {
        const groupsOfModel = byModel.get(model) ?? new Map<string, AccountEntry[]>()
        const list = groupsOfModel.get(group.name) ?? []
        list.push(toEntry(account))
        groupsOfModel.set(group.name, list)
        byModel.set(model, groupsOfModel)
      }
    }
  }
  const rows: ModelRow[] = []
  for (const model of [...byModel.keys()].sort((a, b) => a.localeCompare(b))) {
    const groupsOfModel = byModel.get(model) as Map<string, AccountEntry[]>
    const entries: ModelGroupEntry[] = [...groupsOfModel.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([name, accounts]) => ({ name, accounts: sortAccounts(accounts) }))
    const flat = entries.flatMap((entry) => [entry.name, ...entry.accounts.map((a) => a.name)])
    if (matched([model, ...flat])) rows.push({ model, groups: entries })
  }
  return rows
})

const groupRows = computed<GroupRow[]>(() => {
  const rows: GroupRow[] = []
  for (const group of groups.value) {
    const all = accountsByGroup.value[group.id] ?? []
    const kept = keptAccounts(group.id)
    const byModel = new Map<string, AccountEntry[]>()
    for (const account of kept) {
      for (const model of modelsOf(account)) {
        const list = byModel.get(model) ?? []
        list.push(toEntry(account))
        byModel.set(model, list)
      }
    }
    const models: GroupModelEntry[] = [...byModel.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([model, accounts]) => ({ model, accounts: sortAccounts(accounts) }))
    const flat = models.flatMap((entry) => [entry.model, ...entry.accounts.map((a) => a.name)])
    if (!matched([group.name, group.platform ?? '', ...flat])) continue
    rows.push({
      id: group.id,
      name: group.name,
      platform: group.platform ?? '',
      total: all.length,
      counted: kept.length,
      models,
    })
  }
  return rows
})

const metaText = computed(() => {
  const when = loadedAt.value
    ? t('groupModelAccounts.updatedAt', { time: loadedAt.value.toLocaleTimeString('zh-CN') })
    : t('groupModelAccounts.notLoaded')
  return t('groupModelAccounts.count', { count: modelRows.value.length }) + ' · ' + when
})

function keptAccounts(groupId: number): AccountListItem[] {
  const list = accountsByGroup.value[groupId] ?? []
  return onlyScheduled.value ? list.filter(isScheduled) : list
}

async function fetchGroupAccounts(groupId: number): Promise<AccountListItem[]> {
  const collected: AccountListItem[] = []
  for (let page = 1; ; page += 1) {
    const data = await adminAPI.accounts.list(page, 1000, { group: String(groupId) })
    const items = data.items ?? []
    collected.push(...items)
    if (!items.length || page >= (data.pages || 1)) return collected
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const all = await adminAPI.groups.getAll()
    groups.value = all.filter((group) => group && group.id !== undefined)
    const results = await Promise.all(
      groups.value.map(async (group) => [group.id, await fetchGroupAccounts(group.id)] as const),
    )
    accountsByGroup.value = Object.fromEntries(results)
    loadedAt.value = new Date()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
