<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ChevronRight } from 'lucide-vue-next'
import type { Execution } from '@bindings/github.com/dongwlin/dan-tick/internal/history/models'
import { displayTime, statusClass, statusText } from './presentation'

defineProps<{ items: Execution[] }>()
</script>

<template>
  <table class="w-full min-w-[820px] table-fixed text-left text-sm">
    <thead class="sticky top-0 z-10 bg-surface-solid text-xs text-text-secondary">
      <tr class="h-11 border-b border-border-hairline">
        <th scope="col" class="w-[27%] px-4 font-semibold">任务 / 直播间</th>
        <th scope="col" class="w-[19%] px-3 font-semibold">计划执行</th>
        <th scope="col" class="w-[19%] px-3 font-semibold">实际开始</th>
        <th scope="col" class="w-[16%] px-3 font-semibold">发送结果</th>
        <th scope="col" class="w-[12%] px-3 font-semibold">状态</th>
        <th scope="col" class="w-[7%] px-2"><span class="sr-only">操作</span></th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="item in items"
        :key="item.id"
        class="h-18 border-b border-border-hairline last:border-b-0 hover:bg-surface-muted"
      >
        <td class="px-4">
          <div class="truncate font-semibold text-text-primary" :title="item.task_name_snapshot">
            {{ item.task_name_snapshot }}
          </div>
          <div class="mt-1 text-xs text-text-tertiary">
            直播间 {{ item.platform_room_id_snapshot }}
          </div>
        </td>
        <td class="px-3 text-xs text-text-secondary tabular-nums">
          {{ displayTime(item.scheduled_at) }}
          <span v-if="item.trigger_kind === 'manual'" class="block text-text-tertiary"
            >手动触发</span
          >
        </td>
        <td class="px-3 text-xs text-text-secondary tabular-nums">
          {{ displayTime(item.started_at) }}
        </td>
        <td class="px-3 text-xs tabular-nums">
          <span class="text-soft-success-fg">{{ item.sent_count }} 成功</span>
          <span class="text-text-tertiary"> / {{ item.failed_count }} 失败</span>
        </td>
        <td class="px-3">
          <span class="inline-flex rounded-md px-2 py-1 text-xs" :class="statusClass(item.status)">
            {{ statusText(item.status) }}
          </span>
        </td>
        <td class="px-2 text-center">
          <RouterLink
            :to="{ name: 'history-detail', params: { id: item.id } }"
            :aria-label="`查看 ${item.task_name_snapshot} 的执行详情`"
            class="inline-flex size-10 items-center justify-center rounded-lg text-text-secondary hover:bg-surface-solid-hover hover:text-text-primary"
          >
            <ChevronRight class="size-4" aria-hidden="true" />
          </RouterLink>
        </td>
      </tr>
    </tbody>
  </table>
</template>
