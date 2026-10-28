import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * Regression guard for features_config.web_search_emulation:
 * the channel editor used to render and submit the anthropic dimension only,
 * so any UI save overwrote the other platform dimensions written by the API.
 *
 * FORK-ANCHOR: channel-websearch-platforms-test (二开：平台清单改由
 * credentialsBuilder 的 CHANNEL_WEB_SEARCH_EMULATION_PLATFORMS 维护)
 */
describe('ChannelsView web_search_emulation platform whitelist', () => {
  const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
  const builder = readFileSync(resolve('src/components/account/credentialsBuilder.ts'), 'utf8')
  const zh = readFileSync(resolve('src/i18n/locales/zh/admin/channels.ts'), 'utf8')
  const en = readFileSync(resolve('src/i18n/locales/en/admin/channels.ts'), 'utf8')

  it('declares the whitelist aligned with the backend platform whitelist', () => {
    expect(builder).toContain('export const CHANNEL_WEB_SEARCH_EMULATION_PLATFORMS = [')
    for (const platform of ['anthropic', 'deepseek', 'ag_hub']) {
      expect(builder).toContain(`'${platform}'`)
    }
  })

  it('renders the toggle for every whitelisted platform section', () => {
    expect(source).toContain('supportsChannelWebSearchEmulation(section.platform) && webSearchGlobalEnabled')
    expect(source).not.toContain("section.platform === 'anthropic' && webSearchGlobalEnabled")
  })

  it('submits every whitelisted platform dimension on save', () => {
    const block = source.match(/const wsEmulation[\s\S]*?delete featuresConfig\.web_search_emulation/)?.[0] ?? ''
    expect(block).toContain('supportsChannelWebSearchEmulation(section.platform)')
    expect(block).toContain('wsEmulation[section.platform] = section.enabled ? !!section.web_search_emulation : false')
    expect(block).not.toContain("'anthropic'")
  })

  it('i18n hint no longer hardcodes Anthropic as the only platform', () => {
    expect(zh).not.toContain('所有 Anthropic 分组的账号将自动拦截')
    expect(en).not.toContain("Anthropic groups will intercept")
    expect(zh).toContain('webSearchEmulationHint')
    expect(en).toContain('webSearchEmulationHint')
  })
})
