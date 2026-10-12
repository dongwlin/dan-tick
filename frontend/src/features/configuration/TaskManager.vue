<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { Clock3, ListTodo, Plus, Search, Trash2 } from 'lucide-vue-next'
import { useDeleteTask, useSetTaskEnabled, useTaskPages } from './queries'
import { fieldClass, flatPanel } from '@/components/ui/uiStyles'
import { primaryButton, secondaryButton } from '@/components/ui/buttonStyles'
import UiConfirmDialog from '@/components/ui/UiConfirmDialog.vue'
import { bindingError } from '@/lib/bindingError'
import { useLatestExecutions } from '@/features/operations/queries'
import TaskRunActions from '@/features/operations/TaskRunActions.vue'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import type { Task } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'

const route = useRoute()
const roomID = computed(() => {
  const value = Number(route.query.room)
  return Number.isSafeInteger(value) && value > 0 ? value : 0
})
const search = shallowRef('')
const tasks = useTaskPages(search, roomID)
const enabledMutation = useSetTaskEnabled()
const deleteMutation = useDeleteTask()
const items = computed(() => tasks.data.value?.pages.flatMap((p) => p.items ?? []) ?? [])
const message = shallowRef('')
const pendingDelete = shallowRef<Task | null>(null)
const deleteDescription = computed(() => {
  const task = pendingDelete.value
  if (!task) return ''
  return `删除任务“${task.name}”后无法恢复，已保存的执行历史不会删除。`
})
const latestRuns = useLatestExecutions()
const runsByTask = computed(
  () => new Map((latestRuns.data.value ?? []).map((execution) => [execution.taskId, execution])),
)

function scheduleText(task: Task) {
  const plan = task.schedule
  if (plan.kind === ScheduleKind.Once) return `${plan.localDate} ${plan.localTime}`
  if (plan.kind === ScheduleKind.Daily) return `每天 ${plan.localTime}`
  return `每周 · ${plan.localTime}`
}
async function setEnabled(task: Task, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  try {
    message.value = ''
    await enabledMutation.mutateAsync({ id: task.id, enabled: checked })
  } catch (error) {
    message.value = bindingError(error)
    ;(event.target as HTMLInputElement).checked = task.enabled
  }
}
function requestDelete(task: Task) {
  pendingDelete.value = task
}

function closeDeleteDialog() {
  pendingDelete.value = null
}

async function deleteTask() {
  const task = pendingDelete.value
  if (!task) return
  pendingDelete.value = null
  try {
    message.value = ''
    await deleteMutation.mutateAsync(task.id)
  } catch (error) {
    message.value = bindingError(error)
  }
}
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col gap-4">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1>弹幕任务</h1>
      </div>
      <RouterLink :to="{ name: 'task-create' }" :class="primaryButton">
        <Plus class="size-4" aria-hidden="true" />新建任务
      </RouterLink>
    </header>
    <div :class="[flatPanel, 'flex min-h-0 flex-1 flex-col overflow-hidden']">
      <div
        class="flex flex-wrap items-center justify-between gap-3 border-b border-border-hairline px-5 py-4"
      >
        <h2 class="font-semibold">我的任务</h2>
        <div class="relative max-w-full">
          <Search
            class="pointer-events-none absolute top-3 left-3 size-4 text-text-tertiary"
            aria-hidden="true"
          />
          <span class="sr-only">搜索任务或直播间</span>
          <input
            v-model="search"
            type="search"
            aria-label="搜索任务或直播间"
            :class="fieldClass"
            class="w-65 max-w-full pl-10"
            placeholder="搜索任务或直播间…"
          />
        </div>
      </div>
      <div
        class="min-h-0 flex-1 overflow-y-auto"
        :class="
          !tasks.isPending.value && !tasks.isError.value && !items.length ? 'flex flex-col' : ''
        "
        tabindex="0"
        aria-label="弹幕任务列表"
      >
        <div v-if="tasks.isPending.value" role="status" class="p-6 text-text-secondary">
          正在加载任务…
        </div>
        <div v-else-if="tasks.isError.value" class="p-6" role="alert">
          <p class="text-soft-danger-fg">{{ bindingError(tasks.error.value) }}</p>
          <button type="button" :class="secondaryButton" class="mt-3" @click="tasks.refetch()">
            重试读取
          </button>
        </div>
        <div
          v-else-if="!items.length"
          class="flex min-h-40 flex-1 flex-col items-center justify-center gap-2 p-8 text-center"
        >
          <ListTodo class="size-7 text-text-tertiary" aria-hidden="true" />
          <p class="text-text-secondary">
            {{ search ? '没有找到匹配的任务' : '还没有创建弹幕任务' }}
          </p>
          <button
            v-if="search"
            type="button"
            :class="secondaryButton"
            class="mt-3"
            @click="search = ''"
          >
            清除搜索
          </button>
        </div>
        <template v-else>
          <article v-for="task in items" :key="task.id" class="border-b border-border-hairline p-5">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="min-w-0">
                <h3 class="truncate text-base font-semibold">{{ task.name }}</h3>
                <p class="mt-1 text-xs text-text-tertiary">
                  直播间 #{{ task.roomId }} · {{ scheduleText(task) }} ·
                  {{ task.schedule.timezone }}
                </p>
              </div>
              <RouterLink
                :to="{ name: 'task-edit', params: { id: task.id } }"
                :class="secondaryButton"
                >编辑任务</RouterLink
              >
            </div>
            <div class="mt-4 flex flex-wrap items-center justify-between gap-3">
              <span class="flex items-center gap-2 text-xs text-text-secondary">
                <Clock3 class="size-4" aria-hidden="true" />
                {{
                  task.kind === 'loop'
                    ? `循环 · 每隔 ${task.intervalSeconds} 秒 · 最多 ${task.maxCount ?? '不限'} 次`
                    : '单次发送 · 1 条消息'
                }}
              </span>
              <div class="flex items-center gap-4">
                <div class="flex items-center gap-2 text-xs text-text-secondary">
                  <input
                    type="checkbox"
                    :checked="task.enabled"
                    :disabled="enabledMutation.isPending.value"
                    :aria-label="`${task.name} 自动执行`"
                    @change="setEnabled(task, $event)"
                  />
                  {{ task.enabled ? '自动执行开启' : '自动执行关闭' }}
                </div>
                <button
                  type="button"
                  :disabled="deleteMutation.isPending.value"
                  class="flex items-center gap-1 text-xs text-soft-danger-fg"
                  :aria-label="`删除任务 ${task.name}`"
                  @click="requestDelete(task)"
                >
                  <Trash2 class="size-4" aria-hidden="true" />删除
                </button>
              </div>
            </div>
            <div
              class="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-border-hairline pt-3"
            >
              <TaskRunActions :task-id="task.id" :execution="runsByTask.get(task.id)" />
              <span
                v-if="latestRuns.isError.value"
                class="text-xs text-soft-danger-fg"
                role="alert"
              >
                执行状态读取失败。<button
                  type="button"
                  class="underline"
                  @click="latestRuns.refetch()"
                >
                  重试
                </button>
              </span>
            </div>
          </article>
          <div class="p-5 text-center">
            <button
              v-if="tasks.hasNextPage.value"
              type="button"
              :class="secondaryButton"
              :disabled="tasks.isFetchingNextPage.value"
              :aria-busy="tasks.isFetchingNextPage.value"
              @click="tasks.fetchNextPage()"
            >
              {{ tasks.isFetchingNextPage.value ? '加载中…' : '加载更多' }}
            </button>
            <p v-else class="text-xs text-text-tertiary">已显示全部任务</p>
            <p
              v-if="tasks.isFetchNextPageError.value"
              role="alert"
              class="mt-2 text-soft-danger-fg"
            >
              加载下一页失败。<button
                type="button"
                class="underline"
                @click="tasks.fetchNextPage()"
              >
                重试
              </button>
            </p>
          </div>
        </template>
      </div>
    </div>
    <p v-if="message" role="alert" class="rounded-lg bg-soft-danger-bg p-3 text-soft-danger-fg">
      {{ message }}
    </p>
    <UiConfirmDialog
      :open="pendingDelete !== null"
      title="删除任务"
      :description="deleteDescription"
      confirm-label="删除任务"
      variant="danger"
      @update:open="closeDeleteDialog"
      @confirm="deleteTask"
    />
  </section>
</template>
