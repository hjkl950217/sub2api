import { describe, expect, it } from 'vitest'
import { listPlatforms } from '@/constants/platformCatalog'
import {
  platformAccentColor,
  platformBadgeClass,
  platformBadgeLightClass
} from '../platformColors'

describe('platformColors', () => {
  it('gives every registered platform an explicit badge tone', () => {
    const fallback = platformBadgeClass('__unknown__')
    for (const spec of listPlatforms()) {
      expect(platformBadgeClass(spec.id), spec.id).not.toBe(fallback)
      expect(platformBadgeLightClass(spec.id), spec.id).not.toBe(fallback)
    }
  })

  it('does not reuse the OpenAI or Gemini tone for aggregate hub', () => {
    expect(platformBadgeClass('ag_hub')).not.toBe(platformBadgeClass('openai'))
    expect(platformBadgeClass('ag_hub')).not.toBe(platformBadgeClass('gemini'))
    expect(platformAccentColor('ag_hub')).not.toBe(platformAccentColor('openai'))
    expect(platformAccentColor('ag_hub')).not.toBe(platformAccentColor('gemini'))
  })
})
