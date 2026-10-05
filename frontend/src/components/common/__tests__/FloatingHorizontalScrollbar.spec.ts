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

function defineSize(el: HTMLElement, sizes: Record<string, number>) {
  for (const [key, value] of Object.entries(sizes)) {
    Object.defineProperty(el, key, { value, configurable: true })
  }
}

function buildContainer(scrollWidth: number, clientWidth: number) {
  const container = document.createElement('div')
  const scroller = document.createElement('div')
  scroller.className = 'table-wrapper'
  container.appendChild(scroller)
  defineSize(scroller, { scrollWidth, clientWidth })
  defineScrollLeft(scroller)
  return { container, scroller }
}

/** track 挂在组件内部，jsdom 下量不出宽度，挂载后补一个再让组件重量一次 */
async function mountWithTrackWidth(container: HTMLElement, trackWidth: number) {
  const wrapper = mount(FloatingHorizontalScrollbar, { props: { container } })
  await nextTick()
  const track = wrapper.get('[data-testid="floating-h-scrollbar"]').element as HTMLElement
  defineSize(track, { clientWidth: trackWidth })
  ;(wrapper.vm as unknown as { refresh: () => void }).refresh()
  await nextTick()
  return wrapper
}

describe('FloatingHorizontalScrollbar', () => {
  it('内容溢出时显示滑块，表格滚动时滑块跟着走', async () => {
    const { container, scroller } = buildContainer(1200, 400)
    const wrapper = await mountWithTrackWidth(container, 400)

    const thumb = wrapper.get('[data-testid="floating-h-scrollbar-thumb"]')
    // track 可用宽 392，可视比例 1/3 → 滑块 131，且是「可拖」的样子
    expect(thumb.attributes('style')).toContain('width: 131px')
    expect(thumb.classes()).toContain('cursor-grab')
    expect(wrapper.get('[data-testid="floating-h-scrollbar"]').classes()).not.toContain('opacity-50')

    scroller.scrollLeft = 800
    scroller.dispatchEvent(new Event('scroll'))
    await nextTick()

    // 滚到底 → 滑块贴到右端（travel = 392 - 131 = 261）
    expect(thumb.attributes('style')).toContain('translateX(261px)')
  })

  it('拖动滑块时表格跟着横向滚动', async () => {
    const { container, scroller } = buildContainer(1200, 400)
    const wrapper = await mountWithTrackWidth(container, 400)

    const thumb = wrapper.get('[data-testid="floating-h-scrollbar-thumb"]')
    await thumb.trigger('pointerdown', { clientX: 100 })

    const move = new Event('pointermove')
    Object.defineProperty(move, 'clientX', { value: 361 })
    window.dispatchEvent(move)

    // 拖满 travel（261）→ 表格滚到最右 800
    expect(scroller.scrollLeft).toBe(800)

    window.dispatchEvent(new Event('pointerup'))
  })

  it('内容没溢出时滑块铺满整条并淡显，不给可拖的样子', async () => {
    const { container } = buildContainer(400, 400)
    const wrapper = await mountWithTrackWidth(container, 400)

    const thumb = wrapper.get('[data-testid="floating-h-scrollbar-thumb"]')
    // 可视比 1 → 滑块铺满 track 可用宽
    expect(thumb.attributes('style')).toContain('width: 392px')
    expect(thumb.classes()).toContain('cursor-default')
    expect(wrapper.get('[data-testid="floating-h-scrollbar"]').classes()).toContain('opacity-50')
  })
})
