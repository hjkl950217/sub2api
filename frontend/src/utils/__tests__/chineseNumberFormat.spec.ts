// FORK: 二开用例——数字的中文习惯写法（4 位分组 / 万 / 亿）
import { describe, expect, it } from 'vitest'

import { formatCurrency, formatNumber } from '../format'

describe('formatNumber 中文计数写法', () => {
  it('一万以下直接给数字', () => {
    expect(formatNumber(0)).toBe('0')
    expect(formatNumber(1)).toBe('1')
    expect(formatNumber(999)).toBe('999')
    expect(formatNumber(1234)).toBe('1234')
    expect(formatNumber(9999)).toBe('9999')
  })

  it('一到亿之间折成万，保留两位小数', () => {
    expect(formatNumber(10_000)).toBe('1.00万')
    expect(formatNumber(12_345)).toBe('1.23万')
    expect(formatNumber(123_456)).toBe('12.35万')
    expect(formatNumber(1_234_567)).toBe('123.46万')
    expect(formatNumber(12_345_678)).toBe('1234.57万')
  })

  it('过亿折成亿，保留两位小数', () => {
    expect(formatNumber(100_000_000)).toBe('1.00亿')
    expect(formatNumber(123_456_789)).toBe('1.23亿')
    expect(formatNumber(1_234_567_890)).toBe('12.35亿')
  })

  it('万位四舍五入到 10000.00 万时进位成亿，不出现 10000.00万', () => {
    expect(formatNumber(99_999_999)).toBe('1.00亿')
  })

  it('负数保留符号', () => {
    expect(formatNumber(-12_345)).toBe('-1.23万')
    expect(formatNumber(-999)).toBe('-999')
  })

  it('空值与非法值给 0', () => {
    expect(formatNumber(null)).toBe('0')
    expect(formatNumber(undefined)).toBe('0')
    expect(formatNumber(Number.NaN)).toBe('0')
  })
})

describe('formatCurrency 中文金额写法', () => {
  it('沿用原来的小数精度档位', () => {
    expect(formatCurrency(4.21)).toBe('$4.21')
    expect(formatCurrency(0.0012)).toContain('0.001200')
    expect(formatCurrency(0)).toContain('0.00')
  })

  it('整数部分按 4 位分组，不用 K', () => {
    expect(formatCurrency(1234.5)).toContain('1234.50')
    expect(formatCurrency(12345.67)).toContain('1,2345.67')
    expect(formatCurrency(123456789.5)).toContain('1,2345,6789.50')
  })
})
