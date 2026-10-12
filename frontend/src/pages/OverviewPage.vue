<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { CalendarClock, ChevronRight, CircleAlert, CircleCheck, Plus } from 'lucide-vue-next'
import { useAccountStatus } from '@/features/operations/queries'
import { useOverview, usePreferences } from '@/features/application/queries'
import { bindingError } from '@/features/configuration/queries'
import { flatPanel, primaryButton } from '@/features/configuration/style'
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
  reauthentication_required: '需要重新扫码',
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
    <div v-else-if="overview.isError.value" role="alert" :class="[flatPanel, 'p-6 text-sm']">
      {{ bindingError(overview.error.value) }}
      <button type="button" class="ml-2 underline" @click="overview.refetch()">重试读取</button>
    </div>
    <template v-else-if="overview.data.value">
      <div
        :class="[flatPanel, 'grid shrink-0 grid-cols-2 overflow-hidden min-[851px]:grid-cols-4']"
        aria-label="系统状态"
      >
        <div
          class="flex min-h-22 flex-col justify-center gap-1 border-r border-border-hairline px-[18px] py-3 max-[850px]:border-b"
        >
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
        </div>
        <div
          class="flex min-h-22 flex-col justify-center gap-1 border-r border-border-hairline px-[18px] py-3 max-[850px]:border-r-0 max-[850px]:border-b"
        >
          <span class="text-xs text-text-tertiary">B 站账号</span>
          <strong class="text-lg">{{
            account.isPending.value
              ? '检测中…'
              : account.isError.value
                ? '状态不可用'
                : (statusLabels[account.data.value?.state ?? ''] ?? '暂不可用')
          }}</strong>
        </div>
        <div
          class="flex min-h-22 flex-col justify-center gap-1 border-r border-border-hairline px-[18px] py-3"
        >
          <span class="text-xs text-text-tertiary">已启用任务</span>
          <strong class="text-2xl font-semibold tabular-nums"
            >{{ overview.data.value.enabledTasks }}
            <span class="text-sm font-normal text-text-tertiary"
              >/ {{ overview.data.value.totalTasks }}</span
            ></strong
          >
        </div>
        <div class="flex min-h-22 flex-col justify-center gap-1 px-[18px] py-3">
          <span class="text-xs text-text-tertiary">正在执行 / 排队</span>
          <strong class="text-2xl font-semibold tabular-nums">{{
            overview.data.value.activeExecutions
          }}</strong>
        </div>
      </div>
      <div class="grid min-h-0 max-h-62 gap-4 lg:grid-cols-[minmax(0,1.65fr)_minmax(280px,1fr)]">
        <section
          :class="[flatPanel, 'flex min-h-0 flex-col overflow-hidden']"
          aria-labelledby="upcoming-heading"
        >
          <div class="flex shrink-0 items-center justify-between gap-3 px-5 pt-4 pb-2">
            <h2 id="upcoming-heading" class="font-semibold">即将执行</h2>
            <RouterLink
              to="/tasks"
              class="text-xs text-text-tertiary hover:text-accent-text hover:underline"
              >全部任务 <ChevronRight class="inline size-3" aria-hidden="true"
            /></RouterLink>
          </div>
          <p
            v-if="!preferences.data.value?.schedulingEnabled"
            class="px-5 py-2 text-xs text-soft-warning-fg"
          >
            自动调度已暂停，以下计划目前不会触发。
          </p>
          <ul
            v-if="upcoming.length"
            class="min-h-0 divide-y divide-border-hairline overflow-y-auto px-5"
            aria-label="即将执行的任务"
          >
            <li
              v-for="item in upcoming"
              :key="item.id"
              class="flex items-center justify-between gap-3 py-3"
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
            class="flex min-h-40 flex-1 flex-col items-center justify-center gap-2 p-5 text-center text-sm text-text-secondary"
          >
            <CalendarClock class="size-7 text-text-tertiary" aria-hidden="true" />
            暂无即将执行的任务
          </div>
        </section>
        <section
          :class="[flatPanel, 'flex flex-col overflow-hidden']"
          aria-labelledby="alerts-heading"
        >
          <div class="flex shrink-0 items-center justify-between gap-3 px-5 pt-4 pb-2">
            <h2 id="alerts-heading" class="font-semibold">最近异常</h2>
            <RouterLink
              to="/history"
              class="text-xs text-text-tertiary hover:text-accent-text hover:underline"
              >执行记录 <ChevronRight class="inline size-3" aria-hidden="true"
            /></RouterLink>
          </div>
          <div
            v-if="overview.data.value.latestFailure"
            class="min-h-0 flex-1 space-y-3 overflow-y-auto px-5 pt-2 pb-5"
          >
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
          <div
            v-else
            class="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 p-5 text-center text-sm text-text-secondary"
          >
            <CircleCheck class="size-7 text-text-tertiary" aria-hidden="true" />
            最近没有失败、中断或错过的执行
          </div>
          <div class="shrink-0 px-5 pb-3 text-xs text-text-tertiary">
            <div class="flex items-center justify-between border-t border-border-hairline pt-3">
              <span>已添加直播间</span
              ><RouterLink
                to="/rooms"
                class="inline-flex items-center gap-1 font-semibold text-text-secondary hover:text-accent-text hover:underline"
                >{{ overview.data.value.roomCount }} 个
                <ChevronRight class="inline size-3" aria-hidden="true"
              /></RouterLink>
            </div>
          </div>
        </section>
      </div>
      <section
        :class="[flatPanel, 'flex min-h-35 flex-1 flex-col overflow-hidden']"
        aria-labelledby="recent-heading"
      >
        <div class="flex shrink-0 items-center justify-between gap-3 px-5 pt-4 pb-2">
          <h2 id="recent-heading" class="font-semibold">最近执行</h2>
          <RouterLink
            to="/history"
            class="text-xs text-text-tertiary hover:text-accent-text hover:underline"
            >全部记录 <ChevronRight class="inline size-3" aria-hidden="true"
          /></RouterLink>
        </div>
        <div class="min-h-0 min-w-0 flex-1 overflow-x-auto px-5 pb-2">
          <HistoryTable :items="recent" compact />
        </div>
      </section>
    </template>
  </section>
</template>
