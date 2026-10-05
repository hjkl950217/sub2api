import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountBatchTestModal from '../AccountBatchTestModal.vue'

const { getBatchTestPlan, updateProbedProtocols, showSuccess, showError } = vi.hoisted(() => ({
  getBatchTestPlan: vi.fn(),
  updateProbedProtocols: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getBatchTestPlan,
      updateProbedProtocols
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.textPromptDefault': 'hi',
    'admin.accounts.batchTest.selectedSummary': 'selected-{count}-{eligible}',
    'admin.accounts.batchTest.commonModels': 'common-{count}',
    'admin.accounts.batchTest.passed': 'passed',
    'admin.accounts.batchTest.failed': 'failed',
    'admin.accounts.batchTest.pending': 'pending'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        let value = messages[key] ?? key
        if (params) {
          Object.entries(params).forEach(([name, replacement]) => {
            value = value.replace(`{${name}}`, String(replacement))
          })
        }
        return value
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

// FORK: 二开「批量测试」候选数据
function planFixture() {
  return {
    groups: [
      {
        platform: 'openai',
        account_ids: [1, 2],
        common_models: ['gpt-4o'],
        accounts: [
          { id: 1, name: 'Lanln公益站', models: ['gpt-4o', 'gpt-4.1'], eligible: true },
          { id: 2, name: 'oai2api公益站', models: ['gpt-4o'], eligible: true }
        ]
      },
      {
        platform: 'grok',
        account_ids: [3],
        common_models: [],
        accounts: [
          { id: 3, name: 'Grok账号', models: ['grok-4'], eligible: false, reason: '该平台不支持协议探测' }
        ]
      }
    ],
    skipped_count: 1
  }
}

function mountModal(props: { show: boolean; accountIds: number[] }) {
  return mount(AccountBatchTestModal, {
    props: props as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: { template: '<div class="select-stub"></div>' },
        TextArea: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<textarea class="textarea-stub" :value="modelValue" />'
        },
        Icon: true
      }
    }
  })
}

describe('AccountBatchTestModal', () => {
  beforeEach(() => {
    getBatchTestPlan.mockResolvedValue(planFixture())
    updateProbedProtocols.mockResolvedValue(undefined)
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('打开时拉取候选，按平台分组显示共有模型并标出不可测账号', async () => {
    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getBatchTestPlan).toHaveBeenCalledWith([1, 2, 3])
    expect(wrapper.text()).toContain('selected-3-2')
    expect(wrapper.text()).toContain('common-1')
    expect(wrapper.get('[data-testid="batch-row-1"]').text()).toContain('Lanln公益站')
    expect(wrapper.get('[data-testid="batch-row-3"]').text()).toContain('该平台不支持协议探测')
  })

  it('批量测试按 account_id 分发事件：提交每账号一组参数，逐行更新协议结果与状态', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true,"text":"ok"}\n',
        'data: {"type":"protocol_result","account_id":1,"protocol":"responses","protocol_ok":false}\n',
        'data: {"type":"batch_test_complete","account_id":1,"success":true}\n',
        'data: {"type":"protocol_result","account_id":2,"protocol":"chat_completions","protocol_ok":true}\n',
        'data: {"type":"batch_test_complete","account_id":2,"error":"boom"}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body).targets).toEqual([
      { account_id: 1, model_id: 'gpt-4o', prompt: 'hi' },
      { account_id: 2, model_id: 'gpt-4o', prompt: 'hi' }
    ])

    expect(wrapper.get('[data-testid="batch-row-1"]').text()).toContain('passed')
    expect(wrapper.get('[data-testid="batch-row-2"]').text()).toContain('boom')
  })

  it('更新支持协议时按账号分别写回各自测通的协议', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true}\n',
        'data: {"type":"batch_test_complete","account_id":1,"success":true}\n',
        'data: {"type":"protocol_result","account_id":2,"protocol":"chat_completions","protocol_ok":true}\n',
        'data: {"type":"protocol_result","account_id":2,"protocol":"responses","protocol_ok":true}\n',
        'data: {"type":"batch_test_complete","account_id":2,"success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.find('[data-testid="batch-update-protocols-button"]').exists()).toBe(false)

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="batch-update-protocols-button"]').trigger('click')
    await flushPromises()

    expect(updateProbedProtocols).toHaveBeenCalledWith(1, ['chat_completions'])
    expect(updateProbedProtocols).toHaveBeenCalledWith(2, ['chat_completions', 'responses'])
    expect(wrapper.emitted('updated')).toHaveLength(1)
  })
})
