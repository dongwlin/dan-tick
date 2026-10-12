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
      </main>
    </div>
  </div>
</template>

<style scoped>
@reference '../style.css';

.app-content :deep(h1) {
  @apply mb-2 text-[27px] leading-tight font-semibold tracking-tight;
}
</style>
