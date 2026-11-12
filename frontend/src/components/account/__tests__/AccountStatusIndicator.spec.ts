import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountStatusIndicator from '../AccountStatusIndicator.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      // 模型级限流文案带上插值参数，便于断言模型名确实渲染出来
      t: (key: string, params?: Record<string, unknown>) =>
        key === 'admin.accounts.status.modelRateLimitedUntil' && params
          ? `${params.model} ${key}`
          : key
    })
  }
})

vi.mock('@/utils/format', async () => {
  const actual = await vi.importActual<typeof import('@/utils/format')>('@/utils/format')
  return {
    ...actual,
    formatCountdown: () => '1h'
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'account',
    platform: 'antigravity',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

describe('AccountStatusIndicator', () => {
  it('Claude 5 系列模型限流时显示 Opus 和 Sonnet 的短别名', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          extra: {
            model_rate_limits: {
              'claude-opus-5': {
                rate_limited_at: '2026-07-28T00:00:00Z',
                rate_limit_reset_at: '2099-07-28T00:00:00Z'
              },
              'claude-sonnet-5': {
                rate_limited_at: '2026-07-28T00:00:00Z',
                rate_limit_reset_at: '2099-07-28T00:00:00Z'
              },
              'claude-sonnet-5-5': {
                rate_limited_at: '2026-09-28T00:00:00Z',
                rate_limit_reset_at: '2099-09-28T00:00:00Z'
              }
            }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('COpus5')
    expect(wrapper.text()).toContain('CSon5')
    expect(wrapper.text()).toContain('CSon55')
    expect(wrapper.text()).not.toContain('claude-sonnet-5')
  })

  it('账号级限流时列出具体模型与到点时间，且不再渲染裸 429', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 6,
          name: 'ag-limited',
          rate_limited_at: '2026-11-10T08:00:00Z',
          rate_limit_reset_at: '2099-11-10T12:00:00Z',
          extra: {
            model_rate_limits: {
              'claude-opus-5': {
                rate_limited_at: '2026-11-10T08:00:00Z',
                rate_limit_reset_at: '2099-11-10T12:00:00Z'
              }
            }
          }
        })
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.text()).toContain('COpus5')
    expect(wrapper.text()).toContain('admin.accounts.status.modelRateLimitedUntil')
    expect(wrapper.text()).not.toContain('429')
  })

  it('模型级限流每个模型单独一行，账号级限流只有一行到点时间', () => {
    const modelLevel = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 7,
          name: 'ag-model-limited',
          rate_limited_at: '2026-11-10T08:00:00Z',
          rate_limit_reset_at: '2099-11-10T12:00:00Z',
          extra: {
            model_rate_limits: {
              'claude-opus-5': {
                rate_limited_at: '2026-11-10T08:00:00Z',
                rate_limit_reset_at: '2099-11-10T12:00:00Z'
              },
              'claude-sonnet-5': {
                rate_limited_at: '2026-11-10T08:00:00Z',
                rate_limit_reset_at: '2099-11-10T13:00:00Z'
              }
            }
          }
        })
      },
      global: { stubs: { Icon: true } }
    })

    const modelLines = modelLevel.findAll('.text-amber-600')
    expect(modelLines).toHaveLength(2)
    expect(modelLines[0].text()).toContain('COpus5')
    expect(modelLines[1].text()).toContain('CSon5')

    const accountLevel = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 8,
          name: 'ag-account-limited',
          rate_limited_at: '2026-11-10T08:00:00Z',
          rate_limit_reset_at: '2099-11-10T12:00:00Z'
        })
      },
      global: { stubs: { Icon: true } }
    })

    const accountLines = accountLevel.findAll('.text-amber-600')
    expect(accountLines).toHaveLength(1)
    expect(accountLines[0].text()).toBe('admin.accounts.status.rateLimitedUntilShort')
  })

  it.each([
    ['claude-fable-5-2', 'CFable52'],
    ['claude-mythos-5-2', 'CMythos52']
  ])('shows a short rate-limit badge for %s', (model, alias) => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          platform: 'anthropic',
          extra: {
            model_rate_limits: {
              [model]: {
                rate_limited_at: '2026-11-10T00:00:00Z',
                rate_limit_reset_at: '2099-11-10T00:00:00Z'
              }
            }
          }
        })
      },
      global: { stubs: { Icon: true } }
    })
    expect(wrapper.text()).toContain(alias)
    expect(wrapper.text()).not.toContain(model)
  })

  it('Grok 账号额度限流时显示自动恢复时间而非临时不可调度', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 5,
          name: 'grok-free-1',
          platform: 'grok',
          rate_limited_at: '2026-07-11T12:00:00Z',
          rate_limit_reset_at: '2099-07-11T13:00:00Z',
          temp_unschedulable_until: '2099-07-11T12:30:00Z',
          temp_unschedulable_reason: 'legacy grok rate limited'
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.find('.badge-warning').text()).toBe('admin.accounts.status.rateLimited')
    expect(wrapper.text()).toContain('admin.accounts.status.rateLimitedAutoResume')
    expect(wrapper.text()).not.toContain('admin.accounts.status.tempUnschedulable')
  })

  it('模型限流 + overages 启用 + 无 AICredits key → 显示 ⚡ (credits_active)', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 1,
          name: 'ag-1',
          extra: {
            allow_overages: true,
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z'
              }
            }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('⚡')
    expect(wrapper.text()).toContain('CSon45')
  })

  it('模型限流 + overages 未启用 → 普通限流样式（无 ⚡）', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 2,
          name: 'ag-2',
          extra: {
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z'
              }
            }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('CSon45')
    expect(wrapper.text()).not.toContain('⚡')
  })

  it('AICredits key 生效 → 显示积分已用尽 (credits_exhausted)', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 3,
          name: 'ag-3',
          extra: {
            allow_overages: true,
            model_rate_limits: {
              'AICredits': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z'
              }
            }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('admin.accounts.status.creditsExhausted')
  })

  it('模型限流 + overages 启用 + AICredits key 生效 → 普通限流样式（积分耗尽，无 ⚡）', () => {
    const wrapper = mount(AccountStatusIndicator, {
      props: {
        account: makeAccount({
          id: 4,
          name: 'ag-4',
          extra: {
            allow_overages: true,
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z'
              },
              'AICredits': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z'
              }
            }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    // 模型限流 + 积分耗尽 → 不应显示 ⚡
    expect(wrapper.text()).toContain('CSon45')
    expect(wrapper.text()).not.toContain('⚡')
    // AICredits 积分耗尽状态应显示
    expect(wrapper.text()).toContain('admin.accounts.status.creditsExhausted')
  })
})
