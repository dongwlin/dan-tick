<script setup lang="ts">
import { RouterView, useRoute } from 'vue-router'
import AppSidebar from './AppSidebar.vue'
import AppTitlebar from './AppTitlebar.vue'
import { useBackendEvents } from './useBackendEvents'

useBackendEvents()
const route = useRoute()
</script>

<template>
  <div class="flex h-dvh min-h-0 flex-col overflow-hidden bg-bg-canvas text-text-primary">
    <AppTitlebar />
    <div class="flex min-h-0 min-w-0 flex-1">
      <AppSidebar />
      <main
        id="main"
        class="relative isolate flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
        tabindex="-1"
      >
        <div class="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden="true">
          <div
            class="absolute top-24 -right-28 size-80 rounded-full bg-(--accent-panel) opacity-30"
          ></div>
          <div
            class="absolute bottom-12 -left-24 size-72 rounded-full bg-(--accent-panel-alt) opacity-20"
          ></div>
        </div>
        <div
          class="app-content relative mx-auto flex min-h-0 w-full flex-1 flex-col pt-5 pb-3"
          :class="
            route.name === 'settings'
              ? 'overflow-hidden'
              : 'max-w-[1520px] overflow-y-auto px-7 max-[1100px]:px-5'
          "
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
