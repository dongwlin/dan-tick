<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { CalendarClock, ChevronRight, CircleAlert, Plus } from 'lucide-vue-next'
import { useAccountStatus } from '@/features/operations/queries'
import { useOverview, usePreferences } from '@/features/application/queries'
import { bindingError } from '@/features/configuration/queries'
import { panel, primaryButton } from '@/features/configuration/style'
import { displayTime, statusText } from '@/features/history/presentation'
import HistoryTable from '@/features/history/HistoryTable.vue'

const overview = useOverview()
const account = useAccountStatus()
const preferences = usePreferences()
const upcoming = computed(() => overview.data.value?.upcoming ?? [])
const recent = computed(() => overview.data.value?.recent ?? [])
function scheduledTime(at: number, timezone: string) {
  return new Date(at).toLocaleString('zh-CN', { timeZone: timezone, hour12: false })
}
const statusLabels: Record<string, string> = {
  logged_in: '已登录',
  unconfigured: '未配置',
  expired: '凭证已过期',
  verification_failed: '验证失败',
}
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col gap-4">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1>总览</h1>
      </div>
      <RouterLink to="/tasks/new" :class="primaryButton">
        <Plus class="size-4" aria-hidden="true" />新建任务
      </RouterLink>
    </header>
    <div
      v-if="overview.isPending.value"
      role="status"
      class="rounded-2xl bg-surface-solid p-6 text-sm text-text-secondary"
    >
      正在加载总览…
    </div>
    <div v-else-if="overview.isError.value" role="alert" :class="[panel, 'p-6 text-sm']">
      {{ bindingError(overview.error.value) }}
      <button type="button" class="ml-2 underline" @click="overview.refetch()">重试读取</button>
    </div>
    <template v-else-if="overview.data.value">
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4" aria-label="系统状态">
        <div :class="[panel, 'flex flex-col gap-2 p-4']">
          <span class="text-xs text-text-tertiary">自动调度</span>
          <strong
            class="text-lg"
            :class="
              preferences.data.value?.schedulingEnabled
                ? 'text-soft-success-fg'
                : 'text-soft-warning-fg'
            "
          >
            {{
              preferences.isPending.value
                ? '读取中…'
                : preferences.isError.value
                  ? '状态不可用'
                  : preferences.data.value?.schedulingEnabled
                    ? '运行中'
                    : '已暂停'
            }}
          </strong>
          <RouterLink to="/settings" class="text-xs text-accent-text hover:underline"
            >调度设置 <ChevronRight class="inline size-3" aria-hidden="true"
          /></RouterLink>
        </div>
        <div :class="[panel, 'flex flex-col gap-2 p-4']">
          <span class="text-xs text-text-tertiary">B 站账号</span>
          <strong class="text-lg">{{
            account.isPending.value
              ? '检测中…'
              : account.isError.value
                ? '状态不可用'
                : (statusLabels[account.data.value?.state ?? ''] ?? '暂不可用')
          }}</strong>
          <RouterLink to="/settings" class="text-xs text-accent-text hover:underline"
            >账号设置 <ChevronRight class="inline size-3" aria-hidden="true"
          /></RouterLink>
        </div>
        <div :class="[panel, 'flex flex-col gap-2 p-4']">
          <span class="text-xs text-text-tertiary">已启用任务</span>
          <strong class="text-2xl font-semibold tabular-nums"
            >{{ overview.data.value.enabledTasks }}
            <span class="text-sm font-normal text-text-tertiary"
              >/ {{ overview.data.value.totalTasks }}</span
            ></strong
          >
          <RouterLink to="/tasks" class="text-xs text-accent-text hover:underline"
            >全部任务 <ChevronRight class="inline size-3" aria-hidden="true"
          /></RouterLink>
        </div>
        <div :class="[panel, 'flex flex-col gap-2 p-4']">
          <span class="text-xs text-text-tertiary">正在执行 / 排队</span>
          <strong class="text-2xl font-semibold tabular-nums">{{
            overview.data.value.activeExecutions
          }}</strong>
          <RouterLink to="/history" class="text-xs text-accent-text hover:underline"
            >执行记录 <ChevronRight class="inline size-3" aria-hidden="true"
          /></RouterLink>
        </div>
      </div>
      <div class="grid min-h-0 max-h-62 gap-4 lg:grid-cols-[minmax(0,1.65fr)_minmax(280px,1fr)]">
        <section
          :class="[panel, 'flex min-h-0 flex-col overflow-hidden']"
          aria-labelledby="upcoming-heading"
        >
          <div
            class="flex items-center justify-between gap-3 border-b border-border-hairline px-5 py-4"
          >
            <h2 id="upcoming-heading" class="font-semibold">即将执行</h2>
            <RouterLink
              to="/tasks"
              class="text-xs text-text-tertiary hover:text-accent-text hover:underline"
              >全部任务 <ChevronRight class="inline size-3" aria-hidden="true"
            /></RouterLink>
          </div>
          <p
            v-if="!preferences.data.value?.schedulingEnabled"
            class="border-b border-border-hairline px-5 py-2 text-xs text-soft-warning-fg"
          >
            自动调度已暂停，以下计划目前不会触发。
          </p>
          <ul
            v-if="upcoming.length"
            class="min-h-0 divide-y divide-border-hairline overflow-y-auto"
            aria-label="即将执行的任务"
          >
            <li
              v-for="item in upcoming"
              :key="item.id"
              class="flex items-center justify-between gap-3 px-5 py-3"
            >
              <div class="min-w-0">
                <RouterLink
                  :to="`/tasks/${item.id}/edit`"
                  class="block truncate text-sm font-semibold text-text-primary hover:text-accent-text hover:underline"
                  >{{ item.name }}</RouterLink
                >
                <span class="text-xs text-text-tertiary"
                  >直播间 {{ item.room }} ·
                  {{ item.kind === 'loop' ? '循环发送' : '单次发送' }}</span
                >
              </div>
              <span class="shrink-0 text-right text-xs text-text-secondary tabular-nums"
                >{{ scheduledTime(item.nextDueAt, item.timezone)
                }}<span class="block text-text-tertiary">{{ item.timezone }}</span></span
              >
            </li>
          </ul>
          <div
            v-else
            class="flex min-h-40 flex-col items-center justify-center gap-2 p-5 text-center text-sm text-text-secondary"
          >
            <CalendarClock class="size-7 text-text-tertiary" aria-hidden="true" />
            暂无即将执行的任务
            <RouterLink to="/tasks/new" class="text-xs text-accent-text underline"
              >创建任务</RouterLink
            >
          </div>
        </section>
        <section :class="[panel, 'flex flex-col overflow-hidden']" aria-labelledby="alerts-heading">
          <div
            class="flex items-center justify-between gap-3 border-b border-border-hairline px-5 py-4"
          >
            <h2 id="alerts-heading" class="font-semibold">最近异常</h2>
            <RouterLink
              to="/history"
              class="text-xs text-text-tertiary hover:text-accent-text hover:underline"
              >执行记录 <ChevronRight class="inline size-3" aria-hidden="true"
            /></RouterLink>
          </div>
          <div v-if="overview.data.value.latestFailure" class="flex-1 space-y-3 p-5">
            <span class="flex items-center gap-2 text-sm text-soft-warning-fg"
              ><CircleAlert class="size-4" aria-hidden="true" />{{
                statusText(overview.data.value.latestFailure.status)
              }}</span
            >
            <h3 class="font-semibold wrap-anywhere">
              {{ overview.data.value.latestFailure.taskName }}
            </h3>
            <p class="text-xs text-text-tertiary">
              {{ displayTime(overview.data.value.latestFailure.time) }}
            </p>
            <RouterLink
              :to="`/history/${overview.data.value.latestFailure.id}`"
              class="text-sm text-accent-text underline hover:text-text-primary"
              >查看执行详情</RouterLink
            >
          </div>
          <p v-else class="flex-1 p-5 text-sm text-text-secondary">
            最近没有失败、中断或错过的执行。
          </p>
          <div
            class="flex items-center justify-between border-t border-border-hairline px-5 py-3 text-xs text-text-tertiary"
          >
            <span>已添加直播间</span
            ><RouterLink to="/rooms" class="text-accent-text hover:underline"
              >{{ overview.data.value.roomCount }} 个
              <ChevronRight class="inline size-3" aria-hidden="true"
            /></RouterLink>
          </div>
        </section>
      </div>
      <section
        :class="[panel, 'flex min-h-35 flex-1 flex-col overflow-hidden']"
        aria-labelledby="recent-heading"
      >
        <div
          class="flex items-center justify-between gap-3 border-b border-border-hairline px-5 py-4"
        >
          <h2 id="recent-heading" class="font-semibold">最近执行</h2>
          <RouterLink
            to="/history"
            class="text-xs text-text-tertiary hover:text-accent-text hover:underline"
            >全部记录 <ChevronRight class="inline size-3" aria-hidden="true"
          /></RouterLink>
        </div>
        <div
          v-if="recent.length"
          class="min-h-0 min-w-0 flex-1 overflow-auto"
          tabindex="0"
          aria-label="最近执行列表"
        >
          <HistoryTable :items="recent" />
        </div>
        <p v-else class="p-5 text-sm text-text-secondary">
          还没有执行记录。运行任务后可在此查看结果。
        </p>
      </section>
    </template>
  </section>
</template>
