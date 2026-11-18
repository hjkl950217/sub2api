// FORK: UrlPingButton 的自检（二开）。
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UrlPingButton from '../UrlPingButton.vue'

const { pingUrl } = vi.hoisted(() => ({ pingUrl: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { pingUrl } }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('UrlPingButton', () => {
  beforeEach(() => {
    pingUrl.mockReset()
  })

  it('地址为空时禁用，不发请求', async () => {
    const wrapper = mount(UrlPingButton, { props: { url: '   ' } })

    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    await wrapper.get('button').trigger('click')
    expect(pingUrl).not.toHaveBeenCalled()
  })

  it('成功时显示耗时，并把 null 代理按直连发给后端', async () => {
    pingUrl.mockResolvedValue({ success: true, latency_ms: 123, status_code: 200 })
    const wrapper = mount(UrlPingButton, { props: { url: 'https://example.com', proxyId: null } })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(pingUrl).toHaveBeenCalledWith({ url: 'https://example.com', proxy_id: null })
    expect(wrapper.text()).toContain('123ms')
  })

  it('选中代理时把 proxy_id 一起发给后端', async () => {
    pingUrl.mockResolvedValue({ success: true, latency_ms: 40, status_code: 200 })
    const wrapper = mount(UrlPingButton, { props: { url: 'https://example.com', proxyId: 7 } })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(pingUrl).toHaveBeenCalledWith({ url: 'https://example.com', proxy_id: 7 })
  })

  it('网络失败时显示失败，原因放在 title 里', async () => {
    pingUrl.mockResolvedValue({ success: false, latency_ms: 0, message: 'dial tcp: timeout' })
    const wrapper = mount(UrlPingButton, { props: { url: 'https://example.com', proxyId: null } })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.pingFailed')
    expect(wrapper.get('button').attributes('title')).toContain('dial tcp: timeout')
  })

  it('地址变化后清掉上次结果', async () => {
    pingUrl.mockResolvedValue({ success: true, latency_ms: 88, status_code: 200 })
    const wrapper = mount(UrlPingButton, { props: { url: 'https://example.com', proxyId: null } })

    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('88ms')

    await wrapper.setProps({ url: 'https://example.org' })
    expect(wrapper.text()).toContain('admin.accounts.pingUrl')
  })
})
