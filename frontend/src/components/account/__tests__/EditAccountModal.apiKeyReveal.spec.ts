import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

const { updateAccountMock, revealApiKeyMock, showErrorMock, authIsSimpleMode } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  revealApiKeyMock: vi.fn(),
  showErrorMock: vi.fn(),
  authIsSimpleMode: { value: true }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: showErrorMock, showSuccess: vi.fn(), showInfo: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isSimpleMode() {
      return authIsSimpleMode.value
    }
  })
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      revealApiKey: revealApiKeyMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: vi.fn() }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function account(overrides: Record<string, unknown> = {}) {
  return {
    id: 21,
    name: 'cn',
    notes: '',
    platform: 'anthropic',
    type: 'apikey',
    credentials: { base_url: 'https://example.test' },
    credentials_status: { has_api_key: true },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false,
    ...overrides
  }
}

function mountModal(value = account()) {
  return mount(EditAccountModal, {
    props: { show: true, account: value, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true
      }
    }
  })
}

const apiKeyInput = (wrapper: ReturnType<typeof mountModal>) =>
  wrapper.find('input.font-mono[autocomplete="new-password"]')

// FORK-ANCHOR: test-edit-api-key-reveal (二开：编辑弹窗 API Key 掩码与眼睛看明文)
describe('EditAccountModal API Key 明文查看', () => {
  beforeEach(() => {
    authIsSimpleMode.value = true
    updateAccountMock.mockReset()
    revealApiKeyMock.mockReset()
    showErrorMock.mockReset()
    updateAccountMock.mockResolvedValue(account())
    revealApiKeyMock.mockResolvedValue('sk-fork-plain')
  })

  it('已配置时默认显示 ****，点眼睛后拉明文并展示', async () => {
    const wrapper = mountModal()
    expect((apiKeyInput(wrapper).element as HTMLInputElement).value).toBe('****')
    // 默认不请求明文：只有点眼睛才拉
    expect(revealApiKeyMock).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="api-key-reveal-toggle"]').trigger('click')
    await vi.waitFor(() => expect(revealApiKeyMock).toHaveBeenCalledWith(21))
    await vi.waitFor(() =>
      expect((apiKeyInput(wrapper).element as HTMLInputElement).value).toBe('sk-fork-plain')
    )
    expect((apiKeyInput(wrapper).element as HTMLInputElement).type).toBe('text')
  })

  it('再点眼睛回到掩码，不重复请求明文', async () => {
    const wrapper = mountModal()
    const toggle = () => wrapper.get('[data-testid="api-key-reveal-toggle"]')
    await toggle().trigger('click')
    await vi.waitFor(() =>
      expect((apiKeyInput(wrapper).element as HTMLInputElement).value).toBe('sk-fork-plain')
    )
    await toggle().trigger('click')
    expect((apiKeyInput(wrapper).element as HTMLInputElement).value).toBe('****')
    expect(revealApiKeyMock).toHaveBeenCalledTimes(1)
  })

  it('未配置 API Key 时不显示掩码，眼睛按钮禁用', () => {
    const wrapper = mountModal(account({ credentials_status: {} }))
    expect((apiKeyInput(wrapper).element as HTMLInputElement).value).toBe('')
    expect(wrapper.get('[data-testid="api-key-reveal-toggle"]').attributes('disabled')).toBeDefined()
  })

  it('提交时掩码不被当作新 key 写回', async () => {
    const wrapper = mountModal()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalled())
    const payload = updateAccountMock.mock.calls[0][1] as { credentials: Record<string, unknown> }
    expect(payload.credentials.api_key).toBeUndefined()
  })

  it('上游类型也带眼睛开关', () => {
    const wrapper = mountModal(
      account({ type: 'upstream', platform: 'openai', credentials: { base_url: 'https://up.test' } })
    )
    expect(wrapper.findAll('[data-testid="api-key-reveal-toggle"]').length).toBeGreaterThan(0)
  })
})
