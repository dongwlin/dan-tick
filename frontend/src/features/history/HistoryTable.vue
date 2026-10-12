<script setup lang="ts">
import { ChevronRight, History } from 'lucide-vue-next'
import type { Execution } from '@bindings/github.com/dongwlin/dan-tick/internal/history/models'
import { displayTime, statusClass, statusText } from './presentation'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import UiIconButton from '@/components/ui/UiIconButton.vue'

withDefaults(defineProps<{ items: Execution[]; compact?: boolean }>(), { compact: false })
</script>

<template>
  <table
    class="w-full table-fixed text-left text-sm"
    :class="compact ? 'flex h-full min-h-0 min-w-[720px] flex-col' : 'min-w-[820px]'"
  >
    <thead
      class="text-xs text-text-secondary"
      :class="compact ? 'block shrink-0 pr-[9px]' : 'sticky top-0 z-10 bg-surface-solid'"
    >
      <tr
        class="h-11 border-b border-border-hairline"
        :class="compact ? 'table w-full table-fixed' : ''"
      >
        <th scope="col" class="px-4 font-semibold" :class="compact ? 'w-[30%]' : 'w-[27%]'">
          任务 / 直播间
        </th>
        <th v-if="compact" scope="col" class="w-1/4 px-3 text-right font-semibold tabular-nums">
          执行时间
        </th>
        <th v-else scope="col" class="w-[19%] px-3 text-right font-semibold tabular-nums">
          计划执行
        </th>
        <th v-if="!compact" scope="col" class="w-[19%] px-3 text-right font-semibold tabular-nums">
          实际开始
        </th>
        <th
          scope="col"
          class="px-3 text-right font-semibold tabular-nums"
          :class="compact ? 'w-1/5' : 'w-[16%]'"
        >
          发送结果
        </th>
        <th scope="col" class="px-3 font-semibold" :class="compact ? 'w-[18%]' : 'w-[12%]'">
          状态
        </th>
        <th scope="col" class="w-[7%] px-2"><span class="sr-only">操作</span></th>
      </tr>
    </thead>
    <tbody
      :class="
        compact
          ? [
              'min-h-0 flex-1 overflow-x-hidden overflow-y-auto [scrollbar-gutter:stable]',
              items.length ? 'block' : 'flex flex-col',
            ]
          : ''
      "
      :tabindex="compact ? 0 : undefined"
      :aria-label="compact ? '最近执行列表' : undefined"
    >
      <tr
        v-for="item in items"
        :key="item.id"
        class="h-18 border-b border-border-hairline last:border-b-0 hover:bg-surface-muted"
        :class="compact ? 'table w-full table-fixed' : ''"
      >
        <td class="px-4">
          <div class="truncate font-semibold text-text-primary" :title="item.task_name_snapshot">
            {{ item.task_name_snapshot }}
          </div>
          <div class="mt-1 text-xs text-text-tertiary">
            直播间 {{ item.platform_room_id_snapshot }}
          </div>
        </td>
        <td v-if="compact" class="px-3 text-right text-xs text-text-secondary tabular-nums">
          {{ displayTime(item.started_at ?? item.scheduled_at ?? item.created_at) }}
        </td>
        <td v-else class="px-3 text-right text-xs text-text-secondary tabular-nums">
          {{ displayTime(item.scheduled_at) }}
          <span v-if="item.trigger_kind === 'manual'" class="block text-text-tertiary"
            >手动触发</span
          >
        </td>
        <td v-if="!compact" class="px-3 text-right text-xs text-text-secondary tabular-nums">
          {{ displayTime(item.started_at) }}
        </td>
        <td class="px-3 text-right text-xs tabular-nums">
          <span class="text-soft-success-fg">{{ item.sent_count }} 成功</span>
          <span class="text-text-tertiary"> / {{ item.failed_count }} 失败</span>
        </td>
        <td class="px-3">
          <span class="inline-flex rounded-md px-2 py-1 text-xs" :class="statusClass(item.status)">
            {{ statusText(item.status) }}
          </span>
        </td>
        <td class="px-2 text-center">
          <UiIconButton
            :to="{ name: 'history-detail', params: { id: item.id } }"
            :label="`查看 ${item.task_name_snapshot} 的执行详情`"
          >
            <ChevronRight class="size-4" aria-hidden="true" />
          </UiIconButton>
        </td>
      </tr>
      <tr
        v-if="compact && !items.length"
        class="flex min-h-0 w-full flex-1 items-center justify-center"
      >
        <td colspan="5" class="flex flex-col items-center justify-center p-0 text-center">
          <UiEmptyState :fill="false" :icon="History" title="还没有执行记录" />
        </td>
      </tr>
    </tbody>
  </table>
</template>
