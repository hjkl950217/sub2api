import { describe, expect, it } from 'vitest'
import {
  bulkEditFields,
  bulkEditFieldPayload,
  visibleBulkEditFields,
  type BulkEditField
} from '../bulkEditFieldCatalog'

const numberField: BulkEditField = {
  key: 'demo_number',
  control: 'number',
  labelKey: 'demo.label',
  hintKey: 'demo.hint',
  appliesTo: () => true
}

const selectField: BulkEditField = {
  key: 'demo_select',
  control: 'select',
  labelKey: 'demo.label',
  hintKey: 'demo.hint',
  appliesTo: () => true,
  options: [
    { value: 'a', labelKey: 'demo.a' },
    { value: 'b', labelKey: 'demo.b' }
  ]
}

const textField: BulkEditField = {
  key: 'demo_text',
  control: 'text',
  labelKey: 'demo.label',
  hintKey: 'demo.hint',
  appliesTo: () => true
}

describe('bulkEditFieldCatalog', () => {
  it('可见性按平台 × 类型的交叉积判定', () => {
    const keysFor = (platforms: string[], types: string[]) =>
      visibleBulkEditFields(platforms, types).map((f) => f.key)

    expect(keysFor(['openai'], ['apikey'])).toContain('upstream_request_zstd')
    // openai + oauth：自动暂停类适用（只看平台），压缩请求体不适用（只认 apikey）
    expect(keysFor(['openai'], ['oauth'])).toContain('auto_pause_5h_disabled')
    expect(keysFor(['openai'], ['oauth'])).not.toContain('upstream_request_zstd')
    // 混选时只留下全平台通用的属性，避免把专属键写到别的平台上
    expect(keysFor(['openai', 'grok'], ['apikey', 'oauth'])).toEqual(['upstream_request_id_header'])
    expect(visibleBulkEditFields([], ['apikey'])).toHaveLength(0)
    expect(visibleBulkEditFields(['openai'], [])).toHaveLength(0)
  })

  it('布尔字段把 enabled/disabled 转成 true/false', () => {
    const toggle = bulkEditFields.find((field) => field.key === 'upstream_request_zstd')!
    expect(bulkEditFieldPayload(toggle, 'enabled')).toBe(true)
    expect(bulkEditFieldPayload(toggle, 'disabled')).toBe(false)
  })

  it('数字字段空值或非数字时不写', () => {
    expect(bulkEditFieldPayload(numberField, '12')).toBe(12)
    expect(bulkEditFieldPayload(numberField, ' 7 ')).toBe(7)
    expect(bulkEditFieldPayload(numberField, '')).toBeUndefined()
    expect(bulkEditFieldPayload(numberField, 'abc')).toBeUndefined()
  })

  it('下拉字段空值时不写', () => {
    expect(bulkEditFieldPayload(selectField, 'b')).toBe('b')
    expect(bulkEditFieldPayload(selectField, '')).toBeUndefined()
  })

  it('文本字段去掉首尾空格后写入，全空时不写', () => {
    expect(bulkEditFieldPayload(textField, ' x-request-id ')).toBe('x-request-id')
    expect(bulkEditFieldPayload(textField, '   ')).toBeUndefined()
  })
})
