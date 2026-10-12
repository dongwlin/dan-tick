<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { flatPanel, solidPanel } from '@/components/uiStyles'
import { secondaryButton } from '@/components/buttonStyles'
import { bindingError } from '@/lib/bindingError'
import { attemptResult, displayTime, errorText, statusClass, statusText } from './presentation'
import { useExecutionHistoryDetail } from './queries'

const props = defineProps<{ executionId: number }>()
const valid = computed(() => Number.isSafeInteger(props.executionId) && props.executionId > 0)
const detail = useExecutionHistoryDetail(() => props.executionId)
const execution = computed(() => detail.data.value?.execution)
const attempts = computed(() => detail.data.value?.attempts ?? [])
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col gap-4">
    <header class="flex items-center justify-between gap-3">
      <div>
        <h1>执行详情</h1>
        <p class="text-sm text-text-secondary">
          执行 #{{ valid ? executionId : '无效编号' }} · 历史快照
        </p>
      </div>
      <RouterLink to="/history" :class="secondaryButton">
        <ArrowLeft class="size-4" aria-hidden="true" />返回执行记录
      </RouterLink>
    </header>
    <div v-if="!valid" :class="[flatPanel, 'p-6']" role="alert">无效的执行记录编号。</div>
    <div v-else-if="detail.isPending.value" role="status" :class="[flatPanel, 'p-6']">
      正在加载执行详情…
    </div>
    <div v-else-if="detail.isError.value" :class="[flatPanel, 'p-6']" role="alert">
      <p class="text-soft-danger-fg">{{ bindingError(detail.error.value) }}</p>
      <button type="button" :class="secondaryButton" class="mt-3" @click="detail.refetch()">
        重试读取
      </button>
    </div>
    <template v-else-if="execution">
      <div :class="[flatPanel, 'shrink-0 p-5']">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h2 class="wrap-anywhere text-lg font-semibold">{{ execution.task_name_snapshot }}</h2>
            <p class="mt-1 text-xs text-text-tertiary">
              直播间 {{ execution.platform_room_id_snapshot }} ·
              {{ execution.trigger_kind === 'manual' ? '手动执行' : '计划执行' }}
            </p>
          </div>
          <span class="rounded-md px-2.5 py-1 text-xs" :class="statusClass(execution.status)">{{
            statusText(execution.status)
          }}</span>
        </div>
        <dl class="mt-5 grid grid-cols-2 gap-x-5 gap-y-3 text-sm md:grid-cols-4">
          <div>
            <dt class="text-xs text-text-tertiary">计划执行</dt>
            <dd class="mt-1 tabular-nums">{{ displayTime(execution.scheduled_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-tertiary">创建时间</dt>
            <dd class="mt-1 tabular-nums">{{ displayTime(execution.created_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-tertiary">排队时间</dt>
            <dd class="mt-1 tabular-nums">{{ displayTime(execution.queued_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-tertiary">实际开始</dt>
            <dd class="mt-1 tabular-nums">{{ displayTime(execution.started_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-tertiary">结束时间</dt>
            <dd class="mt-1 tabular-nums">{{ displayTime(execution.finished_at) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-tertiary">成功发送</dt>
            <dd class="mt-1 text-soft-success-fg">{{ execution.sent_count }} 条</dd>
          </div>
          <div>
            <dt class="text-xs text-text-tertiary">失败发送</dt>
            <dd class="mt-1 text-soft-danger-fg">{{ execution.failed_count }} 条</dd>
          </div>
          <div>
            <dt class="text-xs text-text-tertiary">结果说明</dt>
            <dd class="mt-1 wrap-anywhere">{{ errorText(execution.error_code) }}</dd>
          </div>
        </dl>
      </div>
      <div :class="[solidPanel, 'flex min-h-0 flex-1 flex-col overflow-hidden']">
        <div class="shrink-0 border-b border-border-hairline px-5 py-4">
          <h2 class="font-semibold">
            发送尝试 <span class="text-sm text-text-tertiary">{{ attempts.length }}</span>
          </h2>
        </div>
        <div class="min-h-0 flex-1 overflow-auto" tabindex="0" aria-label="发送尝试详情">
          <p v-if="!attempts.length" class="p-6 text-sm text-text-secondary">
            本次执行没有发送尝试。
          </p>
          <ol v-else class="divide-y divide-border-hairline">
            <li v-for="attempt in attempts" :key="attempt.id" class="px-5 py-4">
              <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
                <strong>第 {{ attempt.sequence }} 次尝试 · 消息 {{ attempt.message_no }}</strong>
                <span
                  :class="
                    attempt.result === 'accepted' ? 'text-soft-success-fg' : 'text-text-secondary'
                  "
                  >{{ attemptResult(attempt.result) }}</span
                >
              </div>
              <p class="mt-1 text-xs text-text-tertiary tabular-nums">
                {{ displayTime(attempt.attempted_at) }}
              </p>
              <p class="mt-2 wrap-anywhere whitespace-pre-wrap text-sm text-text-primary">
                {{ attempt.content_snapshot }}
              </p>
              <p v-if="attempt.error_code" class="mt-2 text-xs text-soft-warning-fg">
                {{ errorText(attempt.error_code) }}
              </p>
            </li>
          </ol>
        </div>
      </div>
    </template>
  </section>
</template>
