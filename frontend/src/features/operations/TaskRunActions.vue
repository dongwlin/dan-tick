<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { Play, Square } from 'lucide-vue-next'
import { bindingError } from '@/features/configuration/queries'
import { secondaryButton } from '@/features/configuration/style'
import type { ActiveExecution } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { executionStatus, useStartManual, useStopExecution } from './queries'

const props = defineProps<{ taskId: number; execution?: ActiveExecution }>()
const start = useStartManual()
const stop = useStopExecution()
const feedback = shallowRef('')
const errorMessage = shallowRef('')
const active = computed(() =>
  ['waiting', 'queued', 'running'].includes(props.execution?.status ?? ''),
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
  <div class="flex flex-wrap items-center gap-2 text-xs">
    <span v-if="execution" class="text-text-secondary" aria-live="polite">
      {{ executionStatus[execution.status] ?? '状态未知' }} · 成功 {{ execution.sentCount }} / 失败
      {{ execution.failedCount }}
    </span>
    <button
      v-if="active"
      type="button"
      :class="secondaryButton"
      :disabled="stop.isPending.value"
      :aria-busy="stop.isPending.value"
      :aria-label="`停止任务 ${taskId} 当前执行`"
      @click="cancel"
    >
      <Square class="size-3.5" aria-hidden="true" />{{
        stop.isPending.value ? '停止中…' : '停止执行'
      }}
    </button>
    <button
      v-else
      type="button"
      :class="secondaryButton"
      :disabled="start.isPending.value"
      :aria-busy="start.isPending.value"
      :aria-label="`立即执行任务 ${taskId}`"
      @click="run"
    >
      <Play class="size-3.5" aria-hidden="true" />{{
        start.isPending.value ? '启动中…' : '立即执行'
      }}
    </button>
    <span v-if="feedback" aria-live="polite" class="text-text-secondary">{{ feedback }}</span>
    <span v-if="errorMessage" role="alert" class="text-soft-danger-fg">{{ errorMessage }}</span>
  </div>
</template>
