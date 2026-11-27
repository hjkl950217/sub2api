// FORK: 批量编辑的可批量字段登记表。
// 账号属性越来越多，每加一个都要在 BulkEditAccountModal.vue 里改模板、改 payload、改复位三处，
// 漏一处就出现「批量编辑里没有这个字段」。这张表把属性标记集中到一处：加一行，
// 批量编辑就按所选平台 × 类型自动显示控件、写 payload、复位。
//
// 只登记「一个键、独立可写」的属性。下面这些不能单行登记，需要成组控件，继续留在组件里手工维护
// （依据：EditAccountModal 的保存分支，成对/三态写的键单独写会留下半套配置）：
//   - window_cost_limit ↔ window_cost_sticky_reserve
//   - max_sessions ↔ session_idle_timeout_minutes
//   - enable_tls_fingerprint ↔ tls_fingerprint_profile_id
//   - cache_ttl_override_enabled ↔ cache_ttl_override_target
//   - custom_base_url_enabled ↔ custom_base_url（还要文本控件与「非空才写」规则）
//   - codex_image_generation_bridge ↔ codex_image_generation_explicit_tool_policy（三态互斥，还要清遗留键）
//   - auto_reset_credit_enabled ↔ 5h/7d 阈值
//   - auto_pause_5h/7d_threshold（与各自的 _disabled 是一对，这里只登记 _disabled）
//   - quota_limit / quota_daily_weekly_* 与各自的 reset 字段
//   - upstream_billing_group_sync_enabled / rate_sync_enabled（与 probe_enabled、rate_multiplier 受管联动）
//   - web_search_emulation（还要读全局 web search 开关）
//   - grok_media_eligible（单账号编辑走专用接口，不是 extra 合并）

export type BulkEditFieldControl = 'toggle' | 'number' | 'select' | 'text'

export interface BulkEditFieldBase {
  /** extra 里的键名 */
  key: string
  control: BulkEditFieldControl
  /** 文案键：标题与说明 */
  labelKey: string
  hintKey: string
  /** 该属性对哪些 平台×类型 组合有效；批量编辑按所选组合的交叉积判定，全中才显示 */
  appliesTo: (platform: string, type: string) => boolean
}

export interface BulkEditToggleField extends BulkEditFieldBase {
  control: 'toggle'
}

export interface BulkEditNumberField extends BulkEditFieldBase {
  control: 'number'
  min?: number
  max?: number
}

export interface BulkEditTextField extends BulkEditFieldBase {
  control: 'text'
}

export interface BulkEditSelectField extends BulkEditFieldBase {
  control: 'select'
  /** 选项值即写进 extra 的值；标签走 i18n */
  options: { value: string; labelKey: string }[]
}

export type BulkEditField =
  | BulkEditToggleField
  | BulkEditNumberField
  | BulkEditTextField
  | BulkEditSelectField

const isAnthropicOAuthLike = (platform: string, type: string) =>
  platform === 'anthropic' && (type === 'oauth' || type === 'setup-token')

export const bulkEditFields: BulkEditField[] = [
  {
    key: 'upstream_request_zstd',
    control: 'toggle',
    labelKey: 'admin.accounts.requestZstd.label',
    hintKey: 'admin.accounts.requestZstd.hint',
    appliesTo: (_platform, type) => type === 'apikey'
  },
  {
    key: 'anthropic_passthrough',
    control: 'toggle',
    labelKey: 'admin.accounts.anthropic.apiKeyPassthrough',
    hintKey: 'admin.accounts.anthropic.apiKeyPassthroughDesc',
    appliesTo: (platform, type) => platform === 'anthropic' && type === 'apikey'
  },
  {
    key: 'anthropic_apikey_auth_scheme',
    control: 'select',
    labelKey: 'admin.accounts.anthropic.apiKeyAuthScheme',
    hintKey: 'admin.accounts.anthropic.apiKeyAuthSchemeDesc',
    appliesTo: (platform, type) => platform === 'anthropic' && type === 'apikey',
    options: [
      { value: 'x_api_key', labelKey: 'admin.accounts.anthropic.apiKeyAuthSchemeXApiKey' },
      { value: 'authorization_bearer', labelKey: 'admin.accounts.anthropic.apiKeyAuthSchemeBearer' }
    ]
  },
  {
    key: 'images_url_to_b64_json',
    control: 'toggle',
    labelKey: 'admin.accounts.openai.imagesUrlToB64Json',
    hintKey: 'admin.accounts.openai.imagesUrlToB64JsonDesc',
    appliesTo: (platform, type) => platform === 'openai' && type === 'apikey'
  },
  {
    key: 'auto_pause_5h_disabled',
    control: 'toggle',
    labelKey: 'admin.accounts.autoPause5hDisabled',
    hintKey: 'admin.accounts.autoPauseDisabledHint',
    appliesTo: (platform) => platform === 'openai'
  },
  {
    key: 'auto_pause_7d_disabled',
    control: 'toggle',
    labelKey: 'admin.accounts.autoPause7dDisabled',
    hintKey: 'admin.accounts.autoPauseDisabledHint',
    appliesTo: (platform) => platform === 'openai'
  },
  {
    key: 'session_id_masking_enabled',
    control: 'toggle',
    labelKey: 'admin.accounts.quotaControl.sessionIdMasking.label',
    hintKey: 'admin.accounts.quotaControl.sessionIdMasking.hint',
    appliesTo: isAnthropicOAuthLike
  },
  {
    key: 'upstream_request_id_header',
    control: 'text',
    labelKey: 'admin.accounts.upstreamRequestIdHeader',
    hintKey: 'admin.accounts.upstreamRequestIdHeaderHelp.intro',
    appliesTo: () => true
  }
]

// 表单里控件统一用字符串存值（与项目里其它批量字段的写法一致），写库前在这里转换。
// 返回 undefined 表示「这个值不能用」，payload 里直接跳过该键，不写半套配置。
export function bulkEditFieldPayload(field: BulkEditField, raw: string): unknown {
  switch (field.control) {
    case 'toggle':
      return raw === 'enabled'
    case 'number': {
      const trimmed = raw.trim()
      if (trimmed === '') return undefined
      const parsed = Number(trimmed)
      if (!Number.isFinite(parsed)) return undefined
      return parsed
    }
    case 'text': {
      const trimmed = raw.trim()
      return trimmed === '' ? undefined : trimmed
    }
    case 'select':
      return raw === '' ? undefined : raw
  }
}

// visibleBulkEditFields 取所选平台 × 类型全组合都适用的字段。
// 用交叉积判定偏保守：混选时宁可不显示，也不把某平台的专属键写到别的平台上
// （后端合并 extra 时不校验平台，写错了会留下无效键）。
export function visibleBulkEditFields(platforms: string[], types: string[]): BulkEditField[] {
  if (platforms.length === 0 || types.length === 0) {
    return []
  }
  return bulkEditFields.filter((field) =>
    platforms.every((platform) => types.every((type) => field.appliesTo(platform, type)))
  )
}
