import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import FloatingHorizontalScrollbar from '../FloatingHorizontalScrollbar.vue'

// jsdom 不做布局：scrollWidth/clientWidth 恒为 0、scrollLeft 赋值后读回还是 0。
// 这里手动定义出「表格比容器宽」以及可写的 scrollLeft。
function defineScrollLeft(el: HTMLElement, initial = 0) {
  let value = initial
  Object.defineProperty(el, 'scrollLeft', {
    get: () => value,
    set: (next: number) => {
      value = next
    },
    configurable: true,
  })
}

function buildContainer(scrollWidth: number, clientWidth: number) {
  const container = document.createElement('div')
  const scroller = document.createElement('div')
  scroller.className = 'table-wrapper'
  container.appendChild(scroller)
  Object.defineProperty(scroller, 'scrollWidth', { value: scrollWidth, configurable: true })
  Object.defineProperty(scroller, 'clientWidth', { value: clientWidth, configurable: true })
  defineScrollLeft(scroller)
  return { container, scroller }
}

describe('FloatingHorizontalScrollbar', () => {
  it('内容比容器宽时贴底显示，并把表格的横向滚动同步过来', async () => {
    const { container, scroller } = buildContainer(1200, 400)
    const wrapper = mount(FloatingHorizontalScrollbar, { props: { container } })
    await nextTick()

    const bar = wrapper.get('[data-testid="floating-h-scrollbar"]')
    expect(bar.isVisible()).toBe(true)
    defineScrollLeft(bar.element as HTMLElement)

    scroller.scrollLeft = 300
    scroller.dispatchEvent(new Event('scroll'))
    await nextTick()

    expect((bar.element as HTMLElement).scrollLeft).toBe(300)
  })

  it('内容没溢出时整条不显示', async () => {
    const { container } = buildContainer(400, 400)
    const wrapper = mount(FloatingHorizontalScrollbar, { props: { container } })
    await nextTick()

    expect(wrapper.get('[data-testid="floating-h-scrollbar"]').isVisible()).toBe(false)
  })
})
