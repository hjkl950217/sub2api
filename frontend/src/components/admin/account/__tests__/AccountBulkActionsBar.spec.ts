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

  it('preserves the upstream billing probe action from v0.1.166', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: {
        selectedIds: [1],
        totalResults: 45,
        selectingAll: false,
        allResultsSelected: false
      }
    })

    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.accounts.bulkActions.probeUpstreamBilling')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('probe-upstream-billing')).toHaveLength(1)
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
