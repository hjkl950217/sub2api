// FORK: 二开用例——筛选栏的搜索框只负责把输入交给 AccountsView 自己的 500ms 防抖，
// 不再额外转发 change（见 AGENTS.md 2.9 与锚点 account-search-single-trigger）。
// 上游在 `revert(accounts): restore inline account filters` 里删掉了同名的上游 spec，
// 二开这条断言单独留在本文件，避免下次合并再撞 modify/delete 冲突。
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountTableFilters from '../AccountTableFilters.vue'
import SearchInput from '@/components/common/SearchInput.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('AccountTableFilters（二开）', () => {
  it('搜索只走 update:searchQuery，筛选栏不转发 change', async () => {
    vi.useFakeTimers()
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: 'existing search',
        filters: { platform: '', type: '', status: '', privacy_mode: '', group: '' },
        groups: []
      }
    })

    const search = wrapper.getComponent(SearchInput)
    expect(search.props('modelValue')).toBe('existing search')
    await search.get('input').setValue('new search')

    expect(wrapper.emitted('update:searchQuery')).toEqual([['new search']])
    expect(wrapper.emitted('change')).toBeUndefined()

    // 等过 500ms 防抖窗口，确认没有第二条触发路径
    await vi.advanceTimersByTimeAsync(600)
    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.emitted('update:filters')).toBeUndefined()

    vi.useRealTimers()
  })
})
