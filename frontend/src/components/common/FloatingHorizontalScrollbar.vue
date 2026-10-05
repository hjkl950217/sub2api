<template>
  <!-- FORK: 贴底的横向滚动条。宽表内容多时原生滚动条在表格最底部，得先滚到底才能左右拖；
       这条粘在视口底部，与表格的横向滚动双向同步。 -->
  <div
    v-show="scrollable"
    ref="barRef"
    data-testid="floating-h-scrollbar"
    class="sticky bottom-0 z-30 h-3 overflow-x-auto overflow-y-hidden bg-white/80 backdrop-blur-sm dark:bg-dark-900/80"
    @scroll="onBarScroll"
  >
    <div :style="{ width: `${contentWidth}px` }" class="h-px" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  /** 包着表格的容器，组件在里面找 DataTable 的横向滚动元素 */
  container?: HTMLElement | null
}>()

const barRef = ref<HTMLElement | null>(null)
const scroller = ref<HTMLElement | null>(null)
const contentWidth = ref(0)
const scrollable = ref(false)

let observer: ResizeObserver | null = null

function measure() {
  const el = scroller.value
  if (!el) {
    scrollable.value = false
    return
  }
  contentWidth.value = el.scrollWidth
  // 内容比容器宽 1px 以上才算有横向溢出
  scrollable.value = el.scrollWidth > el.clientWidth + 1
  if (barRef.value && barRef.value.scrollLeft !== el.scrollLeft) {
    barRef.value.scrollLeft = el.scrollLeft
  }
}

function onScrollerScroll() {
  if (!barRef.value || !scroller.value) return
  barRef.value.scrollLeft = scroller.value.scrollLeft
}

function onBarScroll() {
  if (!barRef.value || !scroller.value) return
  // 值相同就直接返回，避免和 onScrollerScroll 来回触发
  if (scroller.value.scrollLeft === barRef.value.scrollLeft) return
  scroller.value.scrollLeft = barRef.value.scrollLeft
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
  }

  measure()
  // 行数据渲染完宽度还会变，下一帧再量一次
  requestAnimationFrame(measure)
}

watch(() => props.container, bind, { immediate: true, flush: 'post' })

onBeforeUnmount(() => {
  scroller.value?.removeEventListener('scroll', onScrollerScroll)
  observer?.disconnect()
  observer = null
})

defineExpose({ refresh: measure })
</script>
