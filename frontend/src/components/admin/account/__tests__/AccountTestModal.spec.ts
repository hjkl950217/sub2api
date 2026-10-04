import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModels, updateProbedProtocols, copyToClipboard, showSuccess, showError } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  updateProbedProtocols: vi.fn(),
  copyToClipboard: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

// FORK: 测试弹窗「更新支持协议」按钮依赖 app store 的提示能力
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels,
      updateProbedProtocols
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.imagePromptDefault': 'Generate a cute orange cat astronaut sticker on a clean pastel background.',
    'admin.accounts.textPromptDefault': '我想使用你，你是什么模型呢？只回复我名字即可'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.imageReceived' && params?.count) {
          return `received-${params.count}`
        }
        if (key === 'admin.accounts.imagePreviewAlt' && params?.index) {
          return `test-image-${params.index}`
        }
        return messages[key] || key
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

function mountModal(account: Record<string, unknown> = {
  id: 42,
  name: 'Gemini Image Test',
  platform: 'gemini',
  type: 'apikey',
  status: 'active'
}) {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: { template: '<div class="select-stub"></div>' },
        TextArea: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<textarea class="textarea-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
        },
        Icon: true
      }
    }
  })
}

describe('AccountTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.0-flash', display_name: 'Gemini 2.0 Flash' },
      { id: 'gemini-2.5-flash-image', display_name: 'Gemini 2.5 Flash Image' },
      { id: 'gemini-3.1-flash-image', display_name: 'Gemini 3.1 Flash Image' }
    ])
    copyToClipboard.mockReset()
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gemini-2.5-flash-image"}\n',
        'data: {"type":"image","image_url":"data:image/png;base64,QUJD","mime_type":"image/png"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('gemini 图片模型测试会携带提示词并渲染图片预览', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('draw a tiny orange cat astronaut')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gemini-3.1-flash-image',
      prompt: 'draw a tiny orange cat astronaut'
    })

    const preview = wrapper.find('img[alt="test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')
  })

  // FORK-ANCHOR: test-modal-text-prompt-input (普通文本测试默认消息可编辑并随请求发送)
  it('普通文本测试默认使用可编辑的中文消息，并提交自定义内容', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.5-flash', display_name: 'Gemini 2.5 Flash' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse(['data: {"type":"test_complete","success":true}\n'])
    ) as any

    const wrapper = mountModal({
      id: 51,
      name: 'Gemini Text Test',
      platform: 'gemini',
      type: 'apikey',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect((promptInput.element as HTMLTextAreaElement).value).toBe('我想使用你，你是什么模型呢？只回复我名字即可')
    await promptInput.setValue('请按自定义提示词回答')
    await (wrapper.vm as any).startTest()
    await flushPromises()

    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body).prompt).toBe('请按自定义提示词回答')
  })

  it('grok 账号测试默认选择 Grok 模型', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'grok-4.3', display_name: 'Grok 4.3' },
      { id: 'grok-build-0.1', display_name: 'Grok Build 0.1' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"grok-4.3"}\n',
        'data: {"type":"content","text":"ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 13,
      name: 'Grok Account',
      platform: 'grok',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'grok-4.3',
      prompt: '我想使用你，你是什么模型呢？只回复我名字即可',
      mode: 'text'
    })
  })

  // FORK-ANCHOR: test-test-modal-sync-protocols (二开：测试弹窗「更新支持协议」按钮)
  it('国产供应商显示「更新支持协议」按钮，且非 CN 平台不显示', async () => {
    const cnWrapper = mountModal({
      id: 7,
      name: 'DeepSeek',
      platform: 'deepseek',
      type: 'apikey',
      status: 'active'
    })
    expect(cnWrapper.find('[data-testid="sync-protocols-button"]').exists()).toBe(true)

    const openaiWrapper = mountModal({
      id: 8,
      name: 'OpenAI',
      platform: 'openai',
      type: 'apikey',
      status: 'active'
    })
    expect(openaiWrapper.find('[data-testid="sync-protocols-button"]').exists()).toBe(false)
  })

  it('点击「更新支持协议」只提交本轮通过结果，不重复测试且不关闭弹窗', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","protocol":"chat_completions","protocol_ok":true,"text":"chat-ok"}\n',
        'data: {"type":"protocol_result","protocol":"anthropic","protocol_ok":false,"text":"anthropic-failed"}\n',
        'data: {"type":"protocol_result","protocol":"responses","protocol_ok":true,"text":"responses-ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 7,
      name: 'DeepSeek',
      platform: 'deepseek',
      type: 'apikey',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).selectedModelId = 'deepseek-chat'
    await (wrapper.vm as any).startTest()
    await flushPromises()
    await wrapper.get('[data-testid="sync-protocols-button"]').trigger('click')
    await flushPromises()

    expect(updateProbedProtocols).toHaveBeenCalledWith(7, ['chat_completions', 'responses'])
    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, testRequest] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(testRequest.body)).not.toHaveProperty('sync_protocols')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.syncProtocolsSuccess')
    expect(wrapper.emitted('protocols-updated')).toHaveLength(1)
    // FORK-ANCHOR: test-modal-protocol-content-assert (二开：各协议返回的正文按 [协议] 前缀分行显示)
    expect(wrapper.text()).toContain('[chat]chat-ok')
    expect(wrapper.text()).toContain('[anthropic]anthropic-failed')
    expect(wrapper.text()).toContain('[responses]responses-ok')
  })

  it('三个协议都没通过时不更新账号，并提示配置未改动', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"protocol_result","protocol":"chat_completions","protocol_ok":false}\n',
        'data: {"type":"protocol_result","protocol":"anthropic","protocol_ok":false}\n',
        'data: {"type":"protocol_result","protocol":"responses","protocol_ok":false}\n',
        'data: {"type":"test_complete"}\n'
      ])
    ) as any
    const wrapper = mountModal({
      id: 7,
      name: 'DeepSeek',
      platform: 'deepseek',
      type: 'apikey',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).selectedModelId = 'deepseek-chat'
    await (wrapper.vm as any).startTest()
    await flushPromises()
    await wrapper.get('[data-testid="sync-protocols-button"]').trigger('click')
    await flushPromises()

    expect(updateProbedProtocols).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.accounts.syncProtocolsNonePassed')
    expect(wrapper.emitted('protocols-updated')).toBeUndefined()
  })

  it('OpenAI Compact 探测会携带 compact 测试模式', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'gpt-5.4', display_name: 'GPT-5.4' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 42,
      name: 'OpenAI OAuth',
      platform: 'openai',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      model_id: 'gpt-5.4',
      prompt: '',
      mode: 'compact'
    })
  })
})
