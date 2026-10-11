<script setup lang="ts">
import { computed, onMounted, onUnmounted, shallowRef, useTemplateRef, watch } from 'vue'
import { ChevronLeft, ChevronRight, History } from 'lucide-vue-next'
import { bindingError } from '@/features/configuration/queries'
import { panel, secondaryButton } from '@/features/configuration/style'
import { historyPageSize } from './pagination'
import { useExecutionHistoryPages } from './queries'
import HistoryTable from './HistoryTable.vue'

const panelElement = useTemplateRef<HTMLElement>('panelElement')
const toolbarElement = useTemplateRef<HTMLElement>('toolbarElement')
const scrollElement = useTemplateRef<HTMLElement>('scrollElement')
const availableHeight = shallowRef(0)
const page = shallowRef(1)
const knownTotal = shallowRef(0)
const pageSize = computed(() => historyPageSize(availableHeight.value, knownTotal.value))
const history = useExecutionHistoryPages(page, pageSize)
const totalPages = computed(() => Math.max(1, Math.ceil(knownTotal.value / pageSize.value)))
const showPagination = computed(() => totalPages.value > 1)
const items = computed(() => history.data.value?.items ?? [])
const isEnd = computed(
  () =>
    items.value.length > 0 &&
    (!showPagination.value ||
      (page.value === totalPages.value && items.value.length < pageSize.value)),
)
const pageNumbers = computed(() => {
  const first = Math.min(Math.max(1, page.value - 2), Math.max(1, totalPages.value - 4))
  return Array.from({ length: Math.min(5, totalPages.value - first + 1) }, (_, i) => first + i)
})

watch(
  () => history.data.value,
  (data) => {
    if (data) knownTotal.value = data.total
  },
  { immediate: true },
)
watch([knownTotal, pageSize], () => {
  if (page.value > totalPages.value) page.value = totalPages.value
})

function changePage(next: number) {
  if (next < 1 || next > totalPages.value || next === page.value) return
  page.value = next
  scrollElement.value?.scrollTo(0, 0)
}

function measure() {
  if (!panelElement.value || !toolbarElement.value) return
  const height = panelElement.value.clientHeight - toolbarElement.value.offsetHeight - 8
  if (height > 0) availableHeight.value = height
}

let observer: ResizeObserver | undefined
onMounted(() => {
  measure()
  window.addEventListener('resize', measure)
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(measure)
    if (panelElement.value) observer.observe(panelElement.value)
    if (toolbarElement.value) observer.observe(toolbarElement.value)
  }
})
onUnmounted(() => {
  observer?.disconnect()
  window.removeEventListener('resize', measure)
})
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col gap-4">
    <header>
      <h1>执行记录</h1>
      <p class="text-sm">查看实际执行结果与每一次发送尝试</p>
    </header>
    <div ref="panelElement" :class="[panel, 'flex min-h-0 flex-1 flex-col overflow-hidden']">
      <div
        ref="toolbarElement"
        class="flex shrink-0 items-center justify-between gap-3 border-b border-border-hairline px-5 py-4"
      >
        <h2 class="font-semibold">
          执行历史
          <span class="ml-1 text-sm text-text-tertiary tabular-nums">{{ knownTotal }}</span>
        </h2>
        <span class="text-xs text-text-tertiary">最近 30 天 · 按执行时间倒序</span>
      </div>
      <div
        ref="scrollElement"
        class="min-h-0 min-w-0 flex-1 overflow-auto"
        tabindex="0"
        aria-label="执行记录表格"
      >
        <div v-if="history.isPending.value" class="p-6 text-text-secondary" role="status">
          正在加载执行记录…
        </div>
        <div v-else-if="history.isError.value" class="p-6" role="alert">
          <p>{{ bindingError(history.error.value) }}</p>
          <button type="button" :class="secondaryButton" class="mt-3" @click="history.refetch()">
            重试读取
          </button>
        </div>
        <div v-else-if="!items.length" class="p-8 text-center">
          <History class="mx-auto mb-3 size-9 text-text-tertiary" aria-hidden="true" />
          <p>{{ knownTotal ? '当前页没有记录' : '还没有执行记录' }}</p>
        </div>
        <template v-else>
          <HistoryTable :items="items" />
          <div
            v-if="isEnd"
            class="mx-4 border-t border-border-hairline py-4 text-center text-xs text-text-tertiary"
          >
            {{
              showPagination
                ? `已到最后一页，共 ${knownTotal} 条记录`
                : `共 ${knownTotal} 条记录，已全部显示`
            }}
          </div>
        </template>
      </div>
      <nav
        v-if="showPagination"
        aria-label="执行记录分页"
        class="flex min-h-[54px] shrink-0 flex-wrap items-center justify-between gap-2 border-t border-border-hairline px-5 py-2"
      >
        <span class="text-xs text-text-tertiary tabular-nums">
          显示 {{ (page - 1) * pageSize + 1 }}–{{ Math.min(knownTotal, page * pageSize) }} 条，共
          {{ knownTotal }} 条
        </span>
        <div class="flex items-center gap-1">
          <button
            type="button"
            class="inline-flex size-9 items-center justify-center rounded-lg hover:bg-surface-muted disabled:opacity-40"
            :disabled="page === 1"
            aria-label="上一页"
            @click="changePage(page - 1)"
          >
            <ChevronLeft class="size-4" aria-hidden="true" />
          </button>
          <button
            v-for="number in pageNumbers"
            :key="number"
            type="button"
            class="inline-flex size-9 items-center justify-center rounded-lg border text-xs tabular-nums hover:bg-surface-muted"
            :class="
              number === page
                ? 'border-soft-accent-border bg-soft-accent-bg text-accent-text'
                : 'border-transparent text-text-secondary'
            "
            :aria-current="number === page ? 'page' : undefined"
            :aria-label="`第 ${number} 页`"
            @click="changePage(number)"
          >
            {{ number }}
          </button>
          <button
            type="button"
            class="inline-flex size-9 items-center justify-center rounded-lg hover:bg-surface-muted disabled:opacity-40"
            :disabled="page === totalPages"
            aria-label="下一页"
            @click="changePage(page + 1)"
          >
            <ChevronRight class="size-4" aria-hidden="true" />
          </button>
        </div>
      </nav>
    </div>
  </section>
</template>
