// FORK-ANCHOR: chart-external-tooltip (二开：图表悬浮提示改渲染到页面级 DOM，避免被画布边界裁切)
import { Chart as ChartJS, Tooltip } from 'chart.js'
import type { Chart, TooltipModel } from 'chart.js'

// 图表悬浮提示改为页面级 DOM，避免 Canvas 内绘制被图表边界裁掉。
// 全站通过 ChartJS.defaults 统一生效，各图表无需单独改配置。

const TOOLTIP_ID = 'chart-external-tooltip'
const OFFSET = 12
const VIEWPORT_MARGIN = 8

function tooltipElement(): HTMLDivElement {
  let el = document.getElementById(TOOLTIP_ID) as HTMLDivElement | null
  if (el) return el
  el = document.createElement('div')
  el.id = TOOLTIP_ID
  el.setAttribute('role', 'tooltip')
  el.style.cssText = [
    'position:fixed',
    'z-index:9999',
    'pointer-events:none',
    'max-width:320px',
    'padding:8px 10px',
    'border-radius:8px',
    'border:1px solid',
    'box-shadow:0 8px 24px rgb(0 0 0 / 0.16)',
    'font-size:12px',
    'line-height:1.5',
    'white-space:normal',
    'word-break:break-word',
    'opacity:0',
    'transition:opacity 0.08s ease'
  ].join(';')
  document.body.appendChild(el)
  return el
}

function hideTooltip(): void {
  const el = document.getElementById(TOOLTIP_ID) as HTMLDivElement | null
  if (el) el.style.opacity = '0'
}

// 与图表内部 tooltip 同样的内容，改成脱离画布的 HTML 渲染，长文本会换行。
export function externalChartTooltip(context: { chart: Chart; tooltip: TooltipModel<any> }): void {
  const { chart, tooltip } = context
  const el = tooltipElement()

  if (!tooltip.opacity) {
    el.style.opacity = '0'
    return
  }

  const dark = document.documentElement.classList.contains('dark')
  el.style.background = dark ? '#1f2937' : '#ffffff'
  el.style.borderColor = dark ? '#374151' : '#e5e7eb'
  el.style.color = dark ? '#e5e7eb' : '#374151'

  el.replaceChildren()

  const title = tooltip.title[0]
  if (title) {
    const titleEl = document.createElement('div')
    titleEl.textContent = title
    titleEl.style.fontWeight = '600'
    titleEl.style.color = dark ? '#f3f4f6' : '#111827'
    if (tooltip.body.length) titleEl.style.marginBottom = '4px'
    el.appendChild(titleEl)
  }

  tooltip.body.forEach((item, index) => {
    const row = document.createElement('div')
    row.style.display = 'flex'
    row.style.alignItems = 'baseline'
    row.style.gap = '6px'

    const color = tooltip.labelColors[index]
    if (color) {
      const dot = document.createElement('span')
      dot.style.cssText = `flex:0 0 8px;width:8px;height:8px;border-radius:50%;background:${color.backgroundColor};border:1px solid ${color.borderColor}`
      row.appendChild(dot)
    }

    const text = document.createElement('span')
    text.textContent = item.lines.join(' ')
    row.appendChild(text)
    el.appendChild(row)
  })

  const rect = chart.canvas.getBoundingClientRect()
  const width = el.offsetWidth
  const height = el.offsetHeight
  const anchorX = rect.left + tooltip.caretX
  const anchorY = rect.top + tooltip.caretY

  // 默认放在光标右下；贴近右/下边缘时翻到另一侧，再夹进视口。
  let left = anchorX + OFFSET
  if (left + width > window.innerWidth - VIEWPORT_MARGIN) {
    left = anchorX - width - OFFSET
  }
  left = Math.max(VIEWPORT_MARGIN, Math.min(left, window.innerWidth - width - VIEWPORT_MARGIN))

  let top = anchorY + OFFSET
  if (top + height > window.innerHeight - VIEWPORT_MARGIN) {
    top = anchorY - height - OFFSET
  }
  top = Math.max(VIEWPORT_MARGIN, Math.min(top, window.innerHeight - height - VIEWPORT_MARGIN))

  el.style.left = `${left}px`
  el.style.top = `${top}px`
  el.style.opacity = '1'
}

// 图表销毁（切页、弹窗关闭）时立刻收起，避免残留浮在页面上。
const tooltipCleanupPlugin = {
  id: 'forkExternalTooltipCleanup',
  beforeDestroy: hideTooltip
}

export function installChartTooltipDefaults(): void {
  // 先注册 tooltip 插件，插件默认值才会出现在 ChartJS.defaults 里。
  ChartJS.register(Tooltip)
  ChartJS.defaults.plugins.tooltip.external = externalChartTooltip as typeof ChartJS.defaults.plugins.tooltip.external
  // 关掉画布内绘制，只走外部 DOM；否则两套提示会同时出现。
  ChartJS.defaults.plugins.tooltip.enabled = false
  // 重复注册同 id 插件是无操作，热更新重跑也安全。
  ChartJS.register(tooltipCleanupPlugin)
}
