<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { Loader2, Play, Square } from 'lucide-vue-next'
import UiIconButton from '@/components/ui/UiIconButton.vue'
import { bindingError } from '@/lib/bindingError'
import type { ActiveExecution } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { executionStatus, useStartManual, useStopExecution } from './queries'

const props = defineProps<{ taskId: number; taskName?: string; execution?: ActiveExecution }>()
const start = useStartManual()
const stop = useStopExecution()
const feedback = shallowRef('')
const errorMessage = shallowRef('')
const active = computed(() =>
  ['waiting', 'queued', 'running'].includes(props.execution?.status ?? ''),
)

const runLabel = computed(() =>
  props.taskName ? `立即执行 ${props.taskName}` : `立即执行任务 ${props.taskId}`,
)
const stopLabel = computed(() =>
  props.taskName ? `停止任务 ${props.taskName} 的当前执行` : `停止任务 ${props.taskId} 当前执行`,
)

// Once the authoritative execution appears, its status text replaces the
// optimistic "已创建执行" feedback.
watch(
  () => props.execution?.id,
  (id) => {
    if (id !== undefined) feedback.value = ''
  },
)

async function run() {
  feedback.value = ''
  errorMessage.value = ''
  try {
    const result = await start.mutateAsync(props.taskId)
    feedback.value = `已创建执行 #${result.executionId}`
  } catch (error) {
    errorMessage.value = bindingError(error)
  }
}

async function cancel() {
  if (!props.execution) return
  errorMessage.value = ''
  feedback.value = ''
  try {
    await stop.mutateAsync(props.execution.id)
    feedback.value = '已请求停止，等待本次发送结束并记录结果。'
  } catch (error) {
    errorMessage.value = bindingError(error)
  }
}
</script>

<template>
  <div class="flex min-w-0 flex-wrap items-center justify-end gap-2 text-xs">
    <span v-if="execution" class="truncate text-text-secondary" aria-live="polite">
      {{ executionStatus[execution.status] ?? '状态未知' }} · 成功 {{ execution.sentCount }} / 失败
      {{ execution.failedCount }}
    </span>
    <span v-if="feedback" class="truncate text-text-secondary" aria-live="polite">{{
      feedback
    }}</span>
    <span v-if="errorMessage" role="alert" class="text-soft-danger-fg">{{ errorMessage }}</span>
    <UiIconButton
      v-if="active"
      variant="danger"
      :label="stopLabel"
      title="停止执行"
      :disabled="stop.isPending.value"
      :aria-busy="stop.isPending.value"
      @click="cancel"
    >
      <Loader2
        v-if="stop.isPending.value"
        class="size-4 motion-safe:animate-spin"
        aria-hidden="true"
      />
      <Square v-else class="size-4" aria-hidden="true" />
    </UiIconButton>
    <UiIconButton
      v-else
      :label="runLabel"
      title="立即执行"
      :disabled="start.isPending.value"
      :aria-busy="start.isPending.value"
      @click="run"
    >
      <Loader2
        v-if="start.isPending.value"
        class="size-4 motion-safe:animate-spin"
        aria-hidden="true"
      />
      <Play v-else class="size-4" aria-hidden="true" />
    </UiIconButton>
  </div>
</template>
