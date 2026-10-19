import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupBadge from '../GroupBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

function mountBadge(platform: string) {
  return mount(GroupBadge, {
    props: { name: 'group', platform },
    global: { stubs: { PlatformIcon: true } },
  })
}

describe('GroupBadge platform tones', () => {
  it('gives aggregate hub its own tone instead of the OpenAI/Gemini fallback', () => {
    const hub = mountBadge('ag_hub')
    const openai = mountBadge('openai')
    const gemini = mountBadge('gemini')

    expect(hub.classes()).toContain('bg-lime-50') // FORK-ANCHOR: ag-hub-color-test (二开：聚合中转改用亮色 lime 配色)
    expect(hub.classes()).not.toEqual(openai.classes())
    expect(hub.classes()).not.toEqual(gemini.classes())
  })

  it.each(['opencode_go', 'typesafe', 'command_code', 'cline'])(
    'has an explicit tone for %s',
    (platform) => {
      const wrapper = mountBadge(platform)
      expect(wrapper.classes()).not.toContain('bg-emerald-100')
      expect(wrapper.classes()).not.toContain('bg-violet-100')
    }
  )
})
