<template>
  <!-- FORK: 贴底的横向滚动条。宽表内容多时原生滚动条在表格最底部，得先滚到底才能左右拖；
       这条粘在视口底部，与表格的横向滚动双向同步。
       滑块是自绘的：全局样式把滚动条定成 8px 高、thumb 默认透明（hover 才显形），
       而 Edge 的 overlay 滚动条又不吃 ::-webkit-scrollbar 的高度定制，原生的根本看不见。 -->
  <div
    ref="trackRef"
    data-testid="floating-h-scrollbar"
    class="sticky bottom-0 z-30 flex h-4 items-center overflow-hidden border-t border-gray-200 bg-white/95 px-1 transition-opacity dark:border-dark-700 dark:bg-dark-900/95"
    :class="scrollable ? '' : 'opacity-50'"
  >
    <div
      data-testid="floating-h-scrollbar-thumb"
      class="h-2 rounded-full bg-gray-400 transition-colors dark:bg-dark-500"
      :class="scrollable
        ? 'cursor-grab hover:bg-gray-500 active:cursor-grabbing dark:hover:bg-dark-400'
        : 'cursor-default'"
      :style="{ width: `${thumbWidth}px`, transform: `translateX(${thumbLeft}px)` }"
      @pointerdown="onThumbPointerDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  /** 包着表格的容器，组件在里面找 DataTable 的横向滚动元素 */
  container?: HTMLElement | null
}>()

const trackRef = ref<HTMLElement | null>(null)
const scroller = ref<HTMLElement | null>(null)
const trackWidth = ref(0)
const contentWidth = ref(0)
const viewWidth = ref(0)
const scrollLeft = ref(0)
const scrollable = ref(false)

let observer: ResizeObserver | null = null

const thumbWidth = computed(() => {
  if (!trackWidth.value || !viewWidth.value || !contentWidth.value) return 0
  const ratio = viewWidth.value / contentWidth.value
  // 太短的滑块不好抓，留个下限；track 本身不够宽时不超出去
  return Math.min(trackWidth.value, Math.max(48, Math.round(trackWidth.value * ratio)))
})

const thumbLeft = computed(() => {
  const travel = trackWidth.value - thumbWidth.value
  const maxScroll = contentWidth.value - viewWidth.value
  if (travel <= 0 || maxScroll <= 0) return 0
  return Math.round(travel * (scrollLeft.value / maxScroll))
})

function measure() {
  const el = scroller.value
  if (!el) {
    scrollable.value = false
    return
  }
  contentWidth.value = el.scrollWidth
  viewWidth.value = el.clientWidth
  scrollLeft.value = el.scrollLeft
  // 内容比容器宽 1px 以上才算有横向溢出
  scrollable.value = el.scrollWidth > el.clientWidth + 1
  trackWidth.value = Math.max(0, (trackRef.value?.clientWidth ?? 0) - 8)
}

function onScrollerScroll() {
  if (scroller.value) scrollLeft.value = scroller.value.scrollLeft
}

function onThumbPointerDown(event: PointerEvent) {
  const el = scroller.value
  if (!el) return
  event.preventDefault()

  const startX = event.clientX
  const startScroll = el.scrollLeft
  const travel = trackWidth.value - thumbWidth.value
  const maxScroll = contentWidth.value - viewWidth.value
  if (travel <= 0 || maxScroll <= 0) return

  const onMove = (moveEvent: PointerEvent) => {
    const delta = ((moveEvent.clientX - startX) / travel) * maxScroll
    el.scrollLeft = startScroll + delta
  }
  const onUp = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }

  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

function bind() {
  observer?.disconnect()
  observer = null
  scroller.value?.removeEventListener('scroll', onScrollerScroll)
  scroller.value = null

  const el = props.container?.querySelector<HTMLElement>('.table-wrapper') ?? null
  if (!el) {
    scrollable.value = false
    return
  }

  scroller.value = el
  el.addEventListener('scroll', onScrollerScroll, { passive: true })

  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(measure)
    observer.observe(el)
    const table = el.querySelector('table')
    if (table) observer.observe(table)
    if (trackRef.value) observer.observe(trackRef.value)
  }

  measure()
  // 行数据渲染完宽度还会变，下一帧再量一次
  requestAnimationFrame(measure)
}

watch(() => props.container, bind, { immediate: true, flush: 'post' })

// 滑块从隐藏变显示时 track 才有宽度，这时补测一次
watch(scrollable, async (visible) => {
  if (!visible) return
  await nextTick()
  measure()
})

onBeforeUnmount(() => {
  scroller.value?.removeEventListener('scroll', onScrollerScroll)
  observer?.disconnect()
  observer = null
})

defineExpose({ refresh: measure })
</script>
