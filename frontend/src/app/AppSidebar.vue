<script setup lang="ts">
import { History, LayoutDashboard, ListTodo, Radio, Settings2 } from 'lucide-vue-next'
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import AccountAvatar from '@/components/ui/AccountAvatar.vue'
import { useAccountStatus } from '@/features/operations/queries'

const route = useRoute()
const account = useAccountStatus()
const currentSection = computed(() => {
  if (route.name === 'task-create' || route.name === 'task-edit') return 'tasks'
  if (route.name === 'history-detail') return 'history'
  return route.name
})

const navigation = [
  { name: 'overview', label: '总览', to: '/', icon: LayoutDashboard },
  { name: 'tasks', label: '弹幕任务', to: '/tasks', icon: ListTodo },
  { name: 'rooms', label: '直播间', to: '/rooms', icon: Radio },
  { name: 'history', label: '执行记录', to: '/history', icon: History },
  { name: 'settings', label: '设置', to: '/settings', icon: Settings2 },
] as const
</script>

<template>
  <aside
    class="relative z-10 flex w-[68px] shrink-0 flex-col border-r border-border-hairline bg-bg-sidebar max-[600px]:w-14"
    aria-label="主导航"
  >
    <nav class="flex flex-col gap-[9px] px-[11px] py-[22px] max-[600px]:px-[5px]" aria-label="页面">
      <RouterLink
        v-for="item in navigation"
        :key="item.name"
        :to="item.to"
        :aria-label="item.label"
        :aria-current="currentSection === item.name ? 'page' : undefined"
        class="group relative flex min-h-[46px] items-center justify-center rounded-[10px] border border-transparent text-text-tertiary transition-colors hover:bg-surface-muted hover:text-text-primary"
        :class="
          currentSection === item.name
            ? 'border-border-hairline bg-surface-muted text-accent-text'
            : ''
        "
      >
        <component :is="item.icon" class="size-[18px]" :stroke-width="1.8" aria-hidden="true" />
        <span
          class="pointer-events-none absolute top-1/2 left-[calc(100%+14px)] z-20 hidden -translate-y-1/2 whitespace-nowrap rounded-[7px] bg-surface-solid-hover px-2.5 py-1.5 text-xs font-medium text-text-primary shadow-(--shadow-overlay) group-hover:block group-focus-visible:block"
          aria-hidden="true"
        >
          {{ item.label }}
        </span>
      </RouterLink>
    </nav>
    <div class="mt-auto px-[11px] pt-3 pb-[22px] max-[600px]:px-[5px]">
      <div
        class="flex min-h-[47px] items-center justify-center p-0.5"
        role="img"
        aria-label="B 站账号头像"
      >
        <AccountAvatar
          :url="
            account.data.value?.state === State.LoggedIn ? account.data.value.avatarUrl : undefined
          "
          class="size-[39px] border-2 border-soft-accent-border"
        />
      </div>
    </div>
  </aside>
</template>
