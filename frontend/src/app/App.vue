<script setup lang="ts">
import { RouterView, useRoute } from 'vue-router'
import { TooltipProvider } from 'reka-ui'
import UiToastHost from '@/components/ui/UiToastHost.vue'
import AppSidebar from './AppSidebar.vue'
import AppTitlebar from './AppTitlebar.vue'
import { useBackendEvents } from './useBackendEvents'

useBackendEvents()
const route = useRoute()
</script>

<template>
  <TooltipProvider :delay-duration="300" :skip-delay-duration="150">
    <div class="flex h-dvh min-h-0 flex-col overflow-hidden bg-bg-canvas text-text-primary">
      <AppTitlebar />
      <div class="flex min-h-0 min-w-0 flex-1">
        <AppSidebar />
        <main id="main" class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden" tabindex="-1">
          <div
            class="app-content mx-auto flex min-h-0 w-full flex-1 flex-col pt-5 pb-3"
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
      <UiToastHost />
    </div>
  </TooltipProvider>
</template>

<style scoped>
@reference '../style.css';

.app-content :deep(h1) {
  @apply mb-2 text-[27px] leading-tight font-semibold tracking-tight;
}
</style>
