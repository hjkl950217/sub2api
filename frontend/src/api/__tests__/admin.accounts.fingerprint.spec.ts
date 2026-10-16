// FORK: 模型指纹检测的前端读取逻辑（二开）。
// runFingerprint 走的是 text/event-stream：后端逐条推进度、最后推结果。
// 这里把 SSE 解析、进度回调、错误事件三条路径都钉住。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { runFingerprint } from '../admin/accounts'

const makeStreamResponse = (chunks: string[]) => {
  const encoder = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      chunks.forEach((chunk) => controller.enqueue(encoder.encode(chunk)))
      controller.close()
    }
  })
  return { ok: true, status: 200, body, text: async () => '' } as unknown as Response
}

const sampleReport = {
  schema: 'fpd-detection-v1',
  analysis: {
    prediction: 'gpt-6-astra',
    prediction_name: 'gpt-6-astra/6.1-sol',
    probability: 0.9,
    results: []
  }
}

describe('runFingerprint 流式读', () => {
  beforeEach(() => {
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

  it('逐个样本回调进度，最后返回报告', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      makeStreamResponse([
        'data: {"type":"progress","completed":1,"total":3}\n\n',
        'data: {"type":"progress","completed":2,"total":3}\n\n',
        `data: ${JSON.stringify({ type: 'result', report: sampleReport })}\n\n`
      ])
    ) as any

    const seen: Array<[number, number]> = []
    const got = await runFingerprint(3354, 'deepseek-v4.1-flash', (done, total) => seen.push([done, total]))

    expect(seen).toEqual([
      [1, 3],
      [2, 3]
    ])
    expect(got).toEqual(sampleReport)
  })

  it('事件被切成任意大小的块也能拼回来', async () => {
    const payload = JSON.stringify({ type: 'result', report: sampleReport })
    const whole = `data: ${payload}\n\n`
    global.fetch = vi.fn().mockResolvedValue(
      // 一刀切在 JSON 中间，模拟网络分包
      makeStreamResponse([whole.slice(0, 12), whole.slice(12)])
    ) as any

    await expect(runFingerprint(1, 'm')).resolves.toEqual(sampleReport)
  })

  it('error 事件变成异常抛出，带上后端给的原因', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      makeStreamResponse(['data: {"type":"error","message":"上游没有返回可用的样本"}\n\n'])
    ) as any

    await expect(runFingerprint(1, 'm')).rejects.toThrow('上游没有返回可用的样本')
  })

  it('流里没有结果时报错，不静默返回空', async () => {
    global.fetch = vi.fn().mockResolvedValue(makeStreamResponse(['data: {"type":"progress","completed":1,"total":3}\n\n'])) as any

    await expect(runFingerprint(1, 'm')).rejects.toThrow()
  })
})
