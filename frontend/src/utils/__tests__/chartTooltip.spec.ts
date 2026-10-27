import { Chart as ChartJS } from 'chart.js'
import { afterEach, describe, expect, it } from 'vitest'
import { externalChartTooltip, installChartTooltipDefaults } from '../chartTooltip'

function fakeContext(opacity: number, title: string, lines: string[]) {
  return {
    chart: { canvas: { getBoundingClientRect: () => ({ left: 0, top: 0 }) } },
    tooltip: {
      opacity,
      title: title ? [title] : [],
      body: [{ lines }],
      labelColors: [{ backgroundColor: '#14b8a6', borderColor: '#14b8a6' }],
      caretX: 100,
      caretY: 100
    }
  } as any
}

describe('chartTooltip', () => {
  afterEach(() => {
    document.getElementById('chart-external-tooltip')?.remove()
  })

  it('关掉画布内提示并挂上外部渲染', () => {
    installChartTooltipDefaults()

    expect(ChartJS.defaults.plugins.tooltip.enabled).toBe(false)
    expect(ChartJS.defaults.plugins.tooltip.external).toBe(externalChartTooltip)
  })

  it('把标题和内容写进页面级元素，模型名不会被当成 HTML', () => {
    externalChartTooltip(fakeContext(1, 'models', ['<b>deepseek-v4.1-flash</b>: 22.46亿 (37.5%)']))

    const el = document.getElementById('chart-external-tooltip')
    expect(el).not.toBeNull()
    expect(el!.style.opacity).toBe('1')
    expect(el!.textContent).toContain('models')
    expect(el!.textContent).toContain('<b>deepseek-v4.1-flash</b>: 22.46亿 (37.5%)')
    expect(el!.querySelector('b')).toBeNull()
  })

  it('透明度为 0 时隐藏', () => {
    externalChartTooltip(fakeContext(1, 'models', ['a: 1']))
    externalChartTooltip(fakeContext(0, '', []))

    expect(document.getElementById('chart-external-tooltip')!.style.opacity).toBe('0')
  })
})
