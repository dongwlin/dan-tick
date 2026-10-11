<script setup lang="ts">
import {
  CalendarClock,
  ClipboardList,
  LayoutDashboard,
  RadioTower,
  Settings2,
} from 'lucide-vue-next'
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <div class="flex min-h-dvh flex-col bg-bg-canvas text-text-primary">
    <!-- Temporary navigation so every route is reachable before the real shell
         exists; F3 replaces it with the sidebar from the approved prototype. -->
    <nav
      class="flex flex-wrap items-center gap-2 border-b border-border-hairline bg-bg-titlebar px-6 py-3"
      aria-label="主导航"
    >
      <RouterLink to="/" class="temp-link" exact-active-class="temp-link--active">
        <LayoutDashboard class="size-4 shrink-0" :stroke-width="1.8" aria-hidden="true" />
        总览
      </RouterLink>
      <RouterLink to="/tasks" class="temp-link" active-class="temp-link--active"
        ><ClipboardList class="size-4 shrink-0" :stroke-width="1.8" aria-hidden="true" />
        弹幕任务</RouterLink
      >
      <RouterLink to="/rooms" class="temp-link" active-class="temp-link--active">
        <RadioTower class="size-4 shrink-0" :stroke-width="1.8" aria-hidden="true" />
        直播间
      </RouterLink>
      <RouterLink to="/history" class="temp-link" active-class="temp-link--active"
        ><CalendarClock class="size-4 shrink-0" :stroke-width="1.8" aria-hidden="true" />
        执行记录</RouterLink
      >
      <RouterLink to="/settings" class="temp-link" active-class="temp-link--active"
        ><Settings2 class="size-4 shrink-0" :stroke-width="1.8" aria-hidden="true" />
        设置</RouterLink
      >
    </nav>
    <main class="app-content flex min-w-0 flex-1 flex-col overflow-auto px-6 py-6">
      <div class="acrylic mx-auto w-full max-w-6xl rounded-[14px] px-7 py-6">
        <RouterView />
      </div>
    </main>
  </div>
</template>

<style scoped>
@reference '../style.css';

.temp-link {
  @apply inline-flex min-h-11 items-center gap-2 rounded-[10px] px-3 py-2.5 text-text-tertiary hover:bg-surface-muted hover:text-text-primary;
}

.temp-link--active {
  @apply bg-surface-muted text-accent-text;
}

.app-content :deep(h1) {
  @apply mb-2 text-2xl font-semibold tracking-tight;
}

.app-content :deep(p) {
  @apply text-text-secondary;
}

.app-content :deep(a:not(.temp-link):not(.ui-button)) {
  @apply text-accent-text underline hover:text-text-primary;
}
</style>
