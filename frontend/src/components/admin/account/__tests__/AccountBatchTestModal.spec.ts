import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountBatchTestModal from '../AccountBatchTestModal.vue'

const { getBatchTestPlan, updateProbedProtocols, setSchedulable, showSuccess, showError } = vi.hoisted(() => ({
  getBatchTestPlan: vi.fn(),
  updateProbedProtocols: vi.fn(),
  setSchedulable: vi.fn(),
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
      updateProbedProtocols,
      setSchedulable
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
    'admin.accounts.batchTest.pending': 'pending',
    'admin.accounts.batchTest.testing': 'testing',
    // FORK-ANCHOR: batch-test-spec-i18n (二开：逐协议行的状态与失败原因文案)
    'admin.accounts.protocolProbePassed': 'passed-protocol',
    'admin.accounts.protocolProbeFailed': 'failed-protocol',
    'admin.accounts.protocolProbeErrorLabel': 'reason:'
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

// holdOpen=true 时流读完后不结束，用来模拟"还有账号在测"的状态。
function createStreamResponse(lines: string[], holdOpen = false) {
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
          if (holdOpen) {
            return new Promise(() => {})
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
          {
            id: 1,
            name: 'Lanln公益站',
            models: ['gpt-4o', 'gpt-4.1'],
            eligible: true,
            schedulable: true,
            protocols: ['chat_completions', 'responses']
          },
          {
            id: 2,
            name: 'oai2api公益站',
            models: ['gpt-4o'],
            eligible: true,
            schedulable: false,
            protocols: ['chat_completions', 'responses']
          }
        ]
      },
      {
        platform: 'grok',
        account_ids: [3],
        common_models: [],
        accounts: [
          {
            id: 3,
            name: 'Grok账号',
            models: ['grok-4'],
            eligible: false,
            reason: '该平台不支持协议探测',
            schedulable: false
          }
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
    setSchedulable.mockResolvedValue({ schedulable: false })
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

    // 按钮常显，没有可写回的行时禁用
    expect(wrapper.get('[data-testid="batch-update-protocols-button"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="batch-update-protocols-button"]').trigger('click')
    await flushPromises()

    expect(updateProbedProtocols).toHaveBeenCalledWith(1, ['chat_completions'])
    expect(updateProbedProtocols).toHaveBeenCalledWith(2, ['chat_completions', 'responses'])
    expect(wrapper.emitted('updated')).toHaveLength(1)
  })

  it('测试进行中也能更新支持协议：只写已经跑完的行', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse(
        [
          'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true}\n',
          'data: {"type":"batch_test_complete","account_id":1,"success":true}\n',
          'data: {"type":"protocol_result","account_id":2,"protocol":"chat_completions","protocol_ok":true}\n'
        ],
        true
      )
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="batch-row-1"]').text()).toContain('passed')
    expect(wrapper.get('[data-testid="batch-row-2"]').text()).toContain('testing')

    await wrapper.get('[data-testid="batch-update-protocols-button"]').trigger('click')
    await flushPromises()

    expect(updateProbedProtocols).toHaveBeenCalledTimes(1)
    expect(updateProbedProtocols).toHaveBeenCalledWith(1, ['chat_completions'])
  })

  it('打开弹窗就按平台铺好协议占位标签（灰色），测试推进时逐个变色', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true}\n',
        'data: {"type":"protocol_result","account_id":1,"protocol":"responses","protocol_ok":false}\n',
        'data: {"type":"batch_test_complete","account_id":1,"success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    // 还没测：两个协议标签已经在，都是灰的
    expect(wrapper.get('[data-testid="batch-protocol-1-chat_completions"]').classes()).toContain('bg-gray-100')
    expect(wrapper.get('[data-testid="batch-protocol-1-responses"]').classes()).toContain('bg-gray-100')

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    // 出结论：通过变绿、失败变红，标签数量不变
    expect(wrapper.get('[data-testid="batch-protocol-1-chat_completions"]').classes()).toContain('bg-green-100')
    expect(wrapper.get('[data-testid="batch-protocol-1-responses"]').classes()).toContain('bg-red-100')
  })

  it('测试返回的正文默认展开，不用再点一下', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true,"text":"hello-body"}\n',
        'data: {"type":"batch_test_complete","account_id":1,"success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(wrapper.get('[data-testid="batch-row-1"]').text()).toContain('hello-body')
  })

  // FORK-ANCHOR: batch-test-single-toggle-spec (二开：一行只留一个展开/收起入口，正文不再着绿色)
  it('同一行不会同时出现「展开」和「收起」，正文用中性色', async () => {
    const longBody = 'x'.repeat(300)
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        `data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true,"text":"${longBody}"}\n`,
        'data: {"type":"batch_test_complete","account_id":1,"success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const row = wrapper.get('[data-testid="batch-row-1"]')
    const texts = row.findAll('button').map((b) => b.text())
    expect(texts.includes('admin.accounts.batchTest.expand') && texts.includes('admin.accounts.batchTest.collapse')).toBe(false)
    // 逐协议正文不再二次折叠
    expect(row.findAll('.line-clamp-2')).toHaveLength(0)

    // 正文不着色：状态色只留在左侧标签上，避免绿色被当成返回值
    const line = row.get('[data-testid="batch-protocol-line-1-chat_completions"]')
    const body = line.findAll('span').at(-1)!
    expect(body.text()).toBe(longBody)
    expect(body.classes()).toContain('text-gray-300')
    expect(body.classes()).not.toContain('text-green-300')
  })

  // FORK-ANCHOR: batch-test-protocol-probing-spec (二开：探测中的标签整套 amber，状态值不再是「待测试」)
  it('探测开始时对应协议标签显示「测试中」，结果回来变结论色', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_probe","account_id":1,"protocol":"chat_completions"}\n',
        'data: {"type":"protocol_probe","account_id":1,"protocol":"responses"}\n',
        'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true,"text":"ok"}\n',
        'data: {"type":"protocol_probe","account_id":2,"protocol":"chat_completions"}\n',
        'data: {"type":"protocol_probe","account_id":2,"protocol":"responses"}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    // 账号 1：已出结论的标签不是测试中，没回结果的标签是
    expect(wrapper.get('[data-testid="batch-protocol-1-chat_completions"]').classes()).toContain('bg-green-100')
    expect(wrapper.get('[data-testid="batch-protocol-1-responses"]').classes()).toContain('bg-amber-100')

    // 账号 2：两个协议都在探测中
    expect(wrapper.get('[data-testid="batch-protocol-2-chat_completions"]').classes()).toContain('bg-amber-100')
    expect(wrapper.get('[data-testid="batch-protocol-2-responses"]').classes()).toContain('bg-amber-100')

    // 逐协议行的状态值显示「测试中」，不再是「待测试」
    const line = wrapper.get('[data-testid="batch-protocol-line-2-chat_completions"]')
    expect(line.text()).toContain('testing')
    expect(line.text()).not.toContain('pending')
  })

  // FORK-ANCHOR: batch-test-protocol-line-spec (二开：一个账号的三个协议各占一行，不挤在同一行)
  it('一个账号的每个协议各占一行，行内显示状态与正文', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":true,"text":"chat-body"}\n',
        'data: {"type":"protocol_result","account_id":1,"protocol":"responses","protocol_ok":false,"error":"responses 404"}\n',
        'data: {"type":"batch_test_complete","account_id":1,"success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    // 每个协议一行，两个协议就是两个独立的行容器
    const chatLine = wrapper.get('[data-testid="batch-protocol-line-1-chat_completions"]')
    const responsesLine = wrapper.get('[data-testid="batch-protocol-line-1-responses"]')
    expect(chatLine.exists()).toBe(true)
    expect(responsesLine.exists()).toBe(true)
    expect(chatLine.element).not.toBe(responsesLine.element)

    // 通过的那行显示正文，失败的那行显示报错
    expect(chatLine.text()).toContain('chat-body')
    expect(chatLine.text()).toContain('passed-protocol')
    expect(responsesLine.text()).toContain('responses 404')
    expect(responsesLine.text()).toContain('failed-protocol')

    // 行容器自身是纵向排布，协议之间才真的换行
    const container = chatLine.element.parentElement as HTMLElement
    expect(container.className).toContain('space-y-1')
  })

  // FORK-ANCHOR: batch-test-failure-spec (二开：失败账号的报错要显示出来，且只显示一次)
  it('失败的协议把完整报错显示在行里，收尾汇总不重复', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","account_id":1,"protocol":"chat_completions","protocol_ok":false,"error":"401 invalid api key"}\n',
        'data: {"type":"protocol_result","account_id":1,"protocol":"responses","protocol_ok":false,"error":"404 not found"}\n',
        'data: {"type":"batch_test_complete","account_id":1,"error":"chat_completions: 401 invalid api key\\nresponses: 404 not found"}\n',
        'data: {"type":"protocol_result","account_id":2,"protocol":"chat_completions","protocol_ok":false,"error":"timeout after 90s"}\n',
        'data: {"type":"batch_test_complete","account_id":2}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    // 账号 1：每个失败协议的报错都在各自的行里
    const row1 = wrapper.get('[data-testid="batch-row-1"]')
    expect(row1.text()).toContain('401 invalid api key')
    expect(row1.text()).toContain('404 not found')
    // FORK-ANCHOR: batch-test-failure-dedup-spec (二开：收尾汇总的是同样的报错，行级红块不重复出现)
    expect(row1.find('[data-testid="batch-failure-1"]').exists()).toBe(false)

    // 账号 2：失败行状态是失败，且逐协议报错可见
    const row2 = wrapper.get('[data-testid="batch-row-2"]')
    expect(row2.text()).toContain('timeout after 90s')
  })

  it('整账号级失败（非协议报错）也显示失败原因', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"batch_test_complete","account_id":1,"error":"测试未返回完成事件，无法判定结果"}\n'
      ])
    ) as any

    const wrapper = mountModal({ show: false, accountIds: [1, 2, 3] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-test-start-button"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const failure = wrapper.get('[data-testid="batch-failure-1"]')
    expect(failure.text()).toContain('测试未返回完成事件')
  })

  it('每行调度开关直接调调度接口，成功后通知外层刷新', async () => {
    const wrapper = mountModal({ show: false, accountIds: [1, 2] })
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="batch-schedulable-1"]').trigger('click')
    await flushPromises()

    expect(setSchedulable).toHaveBeenCalledWith(1, false)
    expect(wrapper.emitted('updated')).toHaveLength(1)
  })
})
