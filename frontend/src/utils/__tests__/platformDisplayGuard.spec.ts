// FORK-ANCHOR: test-platform-display-name-guard (二开：平台名必须走 platformLabel / i18n)
//
// 长空 2026-10-15 定的规矩：**所有 web 上显示的地方都要显示平台名，原始 id 只允许留在代码里**。
// 二开新增的平台（ag_hub 等）在硬编码 map 或裸插值里都会漏成原始 id，所以用这个守卫兜住：
// 扫全部 .vue 模板的 {{ }} 插值，凡是把 platform 字段直接当文本渲染的就报出来。
//
// 覆盖不到的地方（改动这些时得靠人眼）：
//   - script 里拼模板字符串当 label 的，如 `label: \`${g.name} (${g.platform})\``
//   - :title / :label 这类属性绑定（绑定值可能是 id 也可能是显示名，静态判不准）
//   - 通过字符串参数取字段的，如 getDimensionString(row, 'platform')

import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

const SRC = join(process.cwd(), 'src')

// 允许直接渲染平台 id 的地方。长空要求「所有 web 上显示的地方都要改」，所以这里应为空。
// 只放**误报**：表达式里带 platform 但不是平台值的那种。
const ALLOWLIST: string[] = [
  // 这是表头文案对象（columns 是列定义），不是平台值
  'components/channels/AvailableChannelsTable.vue: {{ columns.platform }}'
]

function walkVueFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) {
      if (name === '__tests__' || name === 'node_modules') return []
      return walkVueFiles(full)
    }
    return full.endsWith('.vue') ? [full] : []
  })
}

/** 取 <template> 段里的 {{ ... }} 插值表达式 */
function templateInterpolations(source: string): string[] {
  const start = source.indexOf('<template>')
  const end = source.lastIndexOf('</template>')
  if (start < 0 || end < 0) return []
  const template = source.slice(start, end)
  return [...template.matchAll(/\{\{([\s\S]*?)\}\}/g)].map((m) => m[1].trim())
}

/** 表达式是不是「直接把平台字段当文本渲染」 */
export function isRawPlatformDisplay(expr: string): boolean {
  const raw = expr.trim()
  // 走 helper 的（platformLabel / t / xxxClass）不算
  if (raw.includes('platformLabel(')) return false
  // 直接以函数调用开头的，结果是 helper 的返回值，不算
  if (/^[A-Za-z_$][\w$]*\s*\(/.test(raw)) return false
  const value = raw
    .replace(/(\.\w+\(\))+$/, '') // 去掉尾部方法链，如 .toUpperCase()
    .replace(/\)+$/, '') // 去掉尾部右括号
    .replace(/\s*(\|\||\?\?)\s*['"`][^'"`]*['"`]\s*$/, '') // 去掉 || '兜底'
    .trim()
  return /(?:^|[^\w$])platform$/.test(value)
}

describe('平台名渲染守卫', () => {
  it('模板里不许把原始 platform 字段直接渲染成文本', () => {
    const offenders: string[] = []
    for (const file of walkVueFiles(SRC)) {
      const rel = relative(SRC, file).replace(/\\/g, '/')
      const source = readFileSync(file, 'utf8')
      for (const expr of templateInterpolations(source)) {
        if (!isRawPlatformDisplay(expr)) continue
        const offender = `${rel}: {{ ${expr} }}`
        if (ALLOWLIST.includes(offender)) continue
        offenders.push(offender)
      }
    }
    expect(offenders).toEqual([])
  })
})
