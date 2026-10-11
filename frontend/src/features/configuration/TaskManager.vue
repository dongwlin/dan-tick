<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { Clock3, Plus, Search, Trash2 } from 'lucide-vue-next'
import { bindingError, useDeleteTask, useSetTaskEnabled, useTaskPages } from './queries'
import { fieldClass, panel, primaryButton, secondaryButton } from './style'
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
async function deleteTask(task: Task) {
  if (!window.confirm(`删除任务“${task.name}”？已保存的执行历史不会删除。`)) return
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
        <p class="text-sm">创建任务并安排自动发送计划</p>
      </div>
      <RouterLink :to="{ name: 'task-create' }" :class="primaryButton">
        <Plus class="size-4" aria-hidden="true" />新建任务
      </RouterLink>
    </header>
    <div :class="[panel, 'flex min-h-0 flex-1 flex-col overflow-hidden']">
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
      <div class="min-h-0 flex-1 overflow-y-auto" tabindex="0" aria-label="弹幕任务列表">
        <div v-if="tasks.isPending.value" role="status" class="p-6 text-text-secondary">
          正在加载任务…
        </div>
        <div v-else-if="tasks.isError.value" class="p-6" role="alert">
          <p>{{ bindingError(tasks.error.value) }}</p>
          <button type="button" :class="secondaryButton" class="mt-3" @click="tasks.refetch()">
            重试读取
          </button>
        </div>
        <div v-else-if="!items.length" class="p-8 text-center">
          <p>{{ search ? '没有找到匹配的任务' : '还没有创建弹幕任务' }}</p>
          <button
            v-if="search"
            type="button"
            :class="secondaryButton"
            class="mt-3"
            @click="search = ''"
          >
            清除搜索
          </button>
          <RouterLink v-else to="/tasks/new" :class="secondaryButton" class="mt-3"
            >新建任务</RouterLink
          >
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
                  @click="deleteTask(task)"
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
  </section>
</template>
