import { describe, expect, it } from 'vitest'
import { formatCompactNumber } from '../format'

describe('formatCompactNumber', () => {
  it('跟随 formatNumber 的中文习惯写法', () => {
    expect(formatCompactNumber(0)).toBe('0')
    expect(formatCompactNumber(999)).toBe('999')
    expect(formatCompactNumber(1000)).toBe('1000')
    expect(formatCompactNumber(999_999)).toBe('100.00万')
    expect(formatCompactNumber(1_000_000)).toBe('100.00万')
    expect(formatCompactNumber(1_000_000_000)).toBe('10.00亿')
  })

  it('returns 0 for nullish input', () => {
    expect(formatCompactNumber(null)).toBe('0')
    expect(formatCompactNumber(undefined)).toBe('0')
  })
})
