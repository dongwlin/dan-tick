<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { ListTodo, Pencil, Plus, Search, Trash2 } from 'lucide-vue-next'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { useDeleteTask, useRoomOptions, useSetTaskEnabled, useTaskPages } from './queries'
import { fieldClass } from '@/components/ui/uiStyles'
import { bindingError } from '@/lib/bindingError'
import UiPanel from '@/components/ui/UiPanel.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiAsyncSection from '@/components/ui/UiAsyncSection.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiIconButton from '@/components/ui/UiIconButton.vue'
import { primaryButton, secondaryButton } from '@/components/ui/buttonStyles'
import UiConfirmDialog from '@/components/ui/UiConfirmDialog.vue'
import { useLatestExecutions } from '@/features/operations/queries'
import TaskRunActions from '@/features/operations/TaskRunActions.vue'
import { kindText, nextRunText, roomText, scheduleText } from './presentation'
import type { Task } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'

const route = useRoute()
const roomID = computed(() => {
  const value = Number(route.query.room)
  return Number.isSafeInteger(value) && value > 0 ? value : 0
})
const search = shallowRef('')
const tasks = useTaskPages(search, roomID)
const rooms = useRoomOptions()
const enabledMutation = useSetTaskEnabled()
const deleteMutation = useDeleteTask()
const items = computed(() => tasks.data.value?.pages.flatMap((p) => p.items ?? []) ?? [])
const message = shallowRef('')
const pendingDelete = shallowRef<Task | null>(null)
const enabledOverrides = shallowRef(new Map<number, boolean>())
const deleteDescription = computed(() => {
  const task = pendingDelete.value
  if (!task) return ''
  return `删除任务“${task.name}”后无法恢复，已保存的执行历史不会删除。`
})
const latestRuns = useLatestExecutions()
const runsByTask = computed(
  () => new Map((latestRuns.data.value ?? []).map((execution) => [execution.taskId, execution])),
)

function taskEnabled(task: Task) {
  return enabledOverrides.value.get(task.id) ?? task.enabled
}

async function setEnabled(task: Task, enabled: boolean) {
  enabledOverrides.value = new Map(enabledOverrides.value).set(task.id, enabled)
  try {
    message.value = ''
    await enabledMutation.mutateAsync({ id: task.id, enabled })
  } catch (error) {
    removeEnabledOverride(task.id)
    message.value = bindingError(error)
  }
}

function removeEnabledOverride(id: number) {
  if (!enabledOverrides.value.has(id)) return
  const next = new Map(enabledOverrides.value)
  next.delete(id)
  enabledOverrides.value = next
}

// Drop an optimistic value as soon as the refetched authoritative state agrees.
watch(items, (list) => {
  const overrides = enabledOverrides.value
  if (!overrides.size) return
  let next: Map<number, boolean> | null = null
  for (const task of list) {
    if (overrides.get(task.id) !== task.enabled) continue
    next ??= new Map(overrides)
    next.delete(task.id)
  }
  if (next) enabledOverrides.value = next
})

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
    <UiPageHeader title="弹幕任务">
      <template #actions>
        <RouterLink :to="{ name: 'task-create' }" :class="primaryButton">
          <Plus class="size-4" aria-hidden="true" />新建任务
        </RouterLink>
      </template>
    </UiPageHeader>
    <UiPanel as="div" :padded="false" class="flex min-h-0 flex-1 flex-col overflow-hidden">
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
        <UiAsyncSection
          :pending="tasks.isPending.value"
          :error="tasks.error.value"
          loading-text="正在加载任务…"
          @retry="tasks.refetch()"
        >
          <UiEmptyState
            v-if="!items.length"
            :icon="ListTodo"
            :title="search ? '没有找到匹配的任务' : '还没有创建弹幕任务'"
            :action-label="search ? '清除搜索' : undefined"
            @action="search = ''"
          />
          <template v-else>
            <article
              v-for="task in items"
              :key="task.id"
              class="border-b border-border-hairline px-5 py-4"
            >
              <div
                class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-5 gap-y-3 max-[850px]:grid-cols-1"
              >
                <div class="flex min-w-0 items-center gap-4">
                  <SwitchRoot
                    class="group inline-flex shrink-0 cursor-pointer items-center rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring disabled:cursor-not-allowed disabled:opacity-60"
                    :model-value="taskEnabled(task)"
                    :disabled="enabledMutation.isPending.value"
                    :aria-label="`${task.name} 自动执行`"
                    :title="taskEnabled(task) ? '关闭自动执行' : '开启自动执行'"
                    @update:model-value="(enabled) => setEnabled(task, enabled === true)"
                  >
                    <span
                      aria-hidden="true"
                      class="flex h-[19px] w-[31px] shrink-0 items-center rounded-full border border-border-strong bg-surface-muted px-[2px] group-hover:bg-surface-solid-hover group-data-[state=checked]:border-accent-solid group-data-[state=checked]:bg-accent-solid group-data-[state=checked]:group-hover:bg-accent-solid group-data-[state=checked]:group-hover:brightness-95 group-active:brightness-90 motion-safe:transition-colors motion-safe:duration-200"
                    >
                      <SwitchThumb
                        class="pointer-events-none block size-[13px] rounded-full border border-border-hairline bg-text-tertiary data-[state=checked]:translate-x-3 data-[state=checked]:bg-on-accent motion-safe:transition-[translate,background-color] motion-safe:duration-200"
                      />
                    </span>
                  </SwitchRoot>
                  <div class="min-w-0 flex-1">
                    <h3 class="truncate text-sm font-semibold">{{ task.name }}</h3>
                    <p class="mt-0.5 truncate text-xs text-text-tertiary">
                      {{ roomText(task.roomId, rooms.data.value) }}
                      <span class="mx-1 text-text-disabled" aria-hidden="true">·</span>
                      {{ scheduleText(task) }}
                      <span class="mx-1 text-text-disabled" aria-hidden="true">·</span>
                      <span>{{ kindText(task) }}</span>
                    </p>
                  </div>
                </div>
                <div
                  class="flex min-w-0 flex-wrap items-center justify-end gap-x-5 gap-y-2 max-[850px]:justify-start"
                >
                  <span v-if="latestRuns.isError.value" class="text-soft-danger-fg" role="alert">
                    执行状态读取失败。
                    <button type="button" class="underline" @click="latestRuns.refetch()">
                      重试
                    </button>
                  </span>
                  <div class="w-27 shrink-0 text-right tabular-nums">
                    <span class="block text-xs text-text-tertiary">下次执行</span>
                    <strong class="text-xs font-semibold">{{ nextRunText(task) }}</strong>
                  </div>
                  <div class="flex shrink-0 flex-wrap items-center justify-end gap-1">
                    <TaskRunActions
                      :task-id="task.id"
                      :task-name="task.name"
                      :execution="runsByTask.get(task.id)"
                    />
                    <UiIconButton
                      :to="{ name: 'task-edit', params: { id: task.id } }"
                      :label="`编辑任务 ${task.name}`"
                      title="编辑任务"
                    >
                      <Pencil class="size-4" aria-hidden="true" />
                    </UiIconButton>
                    <UiIconButton
                      variant="danger"
                      :label="`删除任务 ${task.name}`"
                      title="删除任务"
                      :disabled="deleteMutation.isPending.value"
                      @click="requestDelete(task)"
                    >
                      <Trash2 class="size-4" aria-hidden="true" />
                    </UiIconButton>
                  </div>
                </div>
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
              <p v-else class="text-xs text-text-tertiary">
                共 {{ items.length }} 个任务，已全部显示
              </p>
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
        </UiAsyncSection>
      </div>
    </UiPanel>
    <UiAlert v-if="message" role="alert">
      {{ message }}
    </UiAlert>
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
