<script setup lang="ts">
import { RouterView } from 'vue-router'
import AppSidebar from './AppSidebar.vue'
import AppTitlebar from './AppTitlebar.vue'
import { useBackendEvents } from './useBackendEvents'

useBackendEvents()
</script>

<template>
  <div class="flex h-dvh min-h-0 flex-col overflow-hidden bg-bg-canvas text-text-primary">
    <AppTitlebar />
    <div class="flex min-h-0 min-w-0 flex-1">
      <AppSidebar />
      <main id="main" class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden" tabindex="-1">
        <div
          class="app-content mx-auto flex min-h-0 w-full max-w-[1520px] flex-1 flex-col overflow-y-auto px-7 pt-5 pb-3 max-[1100px]:px-5"
        >
          <RouterView />
        </div>
        <footer
          class="mx-auto w-full max-w-[1520px] shrink-0 px-7 pb-2 text-[11px] text-text-tertiary max-[1100px]:px-5"
        >
          dan-tick · 页面功能正在逐步接入
        </footer>
      </main>
    </div>
  </div>
</template>

<style scoped>
@reference '../style.css';

.app-content :deep(h1) {
  @apply mb-2 text-[27px] leading-tight font-semibold tracking-tight;
}

.app-content :deep(p) {
  @apply text-text-secondary;
}

.app-content :deep(a:not(.ui-button)) {
  @apply text-accent-text underline hover:text-text-primary;
}
</style>
