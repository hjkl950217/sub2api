import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import VersionBadge from '../VersionBadge.vue'

// FORK: 二开新增的组件测试——锁定「已是最新」状态下版本下拉里的 fork 仓库链接不消失
const FORK_URL = 'https://github.com/hjkl950217/sub2api'

const appState: any = {
  versionLoading: false,
  currentVersion: '0.2.13',
  latestVersion: '0.2.13',
  hasUpdate: false,
  releaseInfo: { html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.13' },
  buildType: 'release',
  fetchVersion: vi.fn(),
  clearVersionCache: vi.fn()
}

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ isAdmin: true }),
  useAppStore: () => appState
}))
// useClipboard 内部会取真实的 app store，这里一并替换掉
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() })
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

async function mountWithDropdown(state: Record<string, unknown>) {
  Object.assign(
    appState,
    {
      versionLoading: false,
      currentVersion: '0.2.13',
      latestVersion: '0.2.13',
      hasUpdate: false,
      releaseInfo: { html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.13' },
      buildType: 'release'
    },
    state
  )
  const wrapper = mount(VersionBadge, { global: { stubs: { Icon: true } } })
  await wrapper.get('button').trigger('click')
  return wrapper
}

describe('VersionBadge fork 仓库链接', () => {
  it('已是最新（无更新）时也显示 fork 仓库链接', async () => {
    const wrapper = await mountWithDropdown({ hasUpdate: false })
    // 走的是「已是最新」分支：有「查看发布」，链接也要在
    expect(wrapper.text()).toContain('version.viewRelease')
    expect(wrapper.find(`a[href="${FORK_URL}"]`).exists()).toBe(true)
  })

  it('有更新时同样显示 fork 仓库链接', async () => {
    const wrapper = await mountWithDropdown({ hasUpdate: true, latestVersion: '0.2.14' })
    expect(wrapper.find(`a[href="${FORK_URL}"]`).exists()).toBe(true)
  })
})
