import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('AccountBulkActionsBar', () => {
  it('allows selecting all results before any row is selected', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: {
        selectedIds: [],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.accounts.bulkActions.selectAllResults')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('select-all-results')).toHaveLength(1)
  })

  // FORK: 批量启用/停止调度、探测上游倍率、批量刷新令牌四个按钮已按需求移除，这里锁住它们不再出现
  it('hides the removed bulk actions (schedule toggle / billing probe / token refresh)', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: {
        selectedIds: [1],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    const removedKeys = [
      'admin.accounts.bulkActions.probeUpstreamBilling',
      'admin.accounts.bulkActions.refreshToken',
      'admin.accounts.bulkActions.enableScheduling',
      'admin.accounts.bulkActions.disableScheduling'
    ]
    const texts = wrapper.findAll('button').map(item => item.text())
    for (const key of removedKeys) {
      expect(texts.some(text => text.includes(key))).toBe(false)
    }

    // 保留下来的批量操作仍在
    const keptKeys = [
      'admin.accounts.bulkActions.delete',
      'admin.accounts.bulkActions.resetStatus',
      'admin.accounts.bulkActions.edit'
    ]
    for (const key of keptKeys) {
      expect(texts.some(text => text.includes(key))).toBe(true)
    }
  })

  // FORK: 批量测试按钮只在选中里有支持协议探测的平台时出现
  it('shows the batch test action only when the selection supports it', async () => {
    const mountWith = (canBatchTest: boolean) =>
      mount(AccountBulkActionsBar, {
        props: {
          selectedIds: [1],
          totalResults: 45,
          selectingAll: false,
          allResultsSelected: false,
          canBatchTest
        }
      })

    expect(mountWith(false).find('[data-testid="batch-test-button"]').exists()).toBe(false)

    const wrapper = mountWith(true)
    const button = wrapper.get('[data-testid="batch-test-button"]')
    await button.trigger('click')
    expect(wrapper.emitted('batch-test')).toHaveLength(1)
  })
})
