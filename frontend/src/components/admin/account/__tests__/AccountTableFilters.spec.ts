import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import AccountTableFilters from '../AccountTableFilters.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import type { AdminGroup } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: { count: number }) => params ? `${key} (${params.count})` : key
    })
  }
})

const defaultFilters = () => ({
  platform: '', type: '', status: '', privacy_mode: '', group: '',
  search: 'existing search', lite: '1', sort_by: 'priority', sort_order: 'asc'
})

function mountFilters(filters: Record<string, unknown> = defaultFilters()) {
  return mount(AccountTableFilters, {
    attachTo: document.body,
    props: {
      searchQuery: 'existing search', filters,
      groups: [{ id: 42, name: 'A very long group name' } as AdminGroup]
    },
    global: { stubs: { Teleport: true } }
  })
}

enableAutoUnmount(afterEach)
afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('AccountTableFilters', () => {
  it('renders every filter inline without a collapse toggle', () => {
    const filters = { ...defaultFilters(), type: 'bedrock', privacy_mode: '__unset__', group: 'ungrouped' }
    const wrapper = mountFilters(filters)

    // 二开：不再有「更多筛选」折叠面板，五个下拉与搜索框同排常显
    expect(wrapper.find('button[aria-controls]').exists()).toBe(false)
    expect(wrapper.findAllComponents(Select)).toHaveLength(5)
    expect(wrapper.findAllComponents(Select).every(select => select.isVisible())).toBe(true)
    expect(wrapper.props('filters')).toEqual(filters)
    expect(wrapper.emitted('update:filters')).toBeUndefined()
    expect(wrapper.emitted('change')).toBeUndefined()
    wrapper.unmount()
  })

  it('preserves every option and emits a merged filter snapshot plus change for every selection', async () => {
    const filters = { ...defaultFilters(), platform: 'openai', type: 'oauth', status: 'active', privacy_mode: '__unset__', group: '42' }
    const wrapper = mountFilters(filters)
    const expectedOptions = [
      ['platform', ['', ...CONCRETE_PLATFORM_OPTIONS.map(option => option.value)]],
      ['status', ['', 'active', 'inactive', 'error', 'rate_limited', 'temp_unschedulable', 'unschedulable']],
      ['type', ['', 'oauth', 'setup-token', 'apikey', 'bedrock']],
      ['privacy_mode', ['', '__unset__', 'training_off', 'training_set_cf_blocked', 'training_set_failed']],
      ['group', ['', 'ungrouped', '42']]
    ] as const
    let changes = 0
    for (const [index, [key, values]] of expectedOptions.entries()) {
      const select = wrapper.findAllComponents(Select)[index]
      expect(select.props('options').map(option => option.value)).toEqual(values)
      expect(select.props('modelValue')).toBe(filters[key])
      expect(select.get('button').attributes('aria-label')).not.toBe('Select option')
      for (const [optionIndex, value] of values.entries()) {
        await select.get('button').trigger('click')
        await select.findAll('[role="option"]')[optionIndex].trigger('click')
        changes += 1
        expect(wrapper.emitted('update:filters')?.at(-1)).toEqual([{ ...filters, [key]: value }])
        expect(wrapper.emitted('change')).toHaveLength(changes)
        expect(wrapper.props('filters')).toEqual(filters)
      }
    }
    expect(wrapper.findAllComponents(Select)[4].props('options')[2].label).toBe('A very long group name')
    wrapper.unmount()
  })

  it('preserves immediate search updates without emitting a change event', async () => {
    vi.useFakeTimers()
    const wrapper = mountFilters()
    const search = wrapper.getComponent(SearchInput)
    expect(search.props('modelValue')).toBe('existing search')
    expect(search.props('placeholder')).toBe('admin.accounts.searchAccounts')
    await search.get('input').setValue('new search')
    expect(wrapper.emitted('update:searchQuery')).toEqual([['new search']])
    expect(wrapper.emitted('change')).toBeUndefined()
    // FORK-ANCHOR: test-account-search-single-trigger (二开：搜索只走 AccountsView 自己的 500ms 防抖，筛选栏不再转发 change)
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.emitted('update:filters')).toBeUndefined()
    wrapper.unmount()
  })

  it('uses shrinkable controls with desktop widths that keep the whole bar on one row', () => {
    const wrapper = mountFilters()
    expect(wrapper.classes()).toContain('min-w-0')
    expect(wrapper.get('.grid').classes()).toEqual(expect.arrayContaining(['grid-cols-2', 'sm:flex-wrap']))
    for (const select of wrapper.findAllComponents(Select)) {
      expect(select.classes()).toEqual(expect.arrayContaining(['min-w-0', 'w-full']))
      // 二开：一行铺开五个下拉，宽度按最长选项定过，合计不超过 969px 的筛选区
      expect(select.classes().some(cls => /^sm:w-(36|44)$/.test(cls))).toBe(true)
    }
    const widths = wrapper.findAllComponents(Select).map(select =>
      Number(select.classes().find(cls => cls.startsWith('sm:w-'))?.slice(5))
    )
    expect(widths.slice().sort((a, b) => a - b)).toEqual([36, 36, 36, 36, 44])
    wrapper.unmount()
  })
})
