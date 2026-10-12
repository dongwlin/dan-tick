<script setup lang="ts">
import { ToastPortal, ToastProvider, ToastViewport } from 'reka-ui'
import UiToast from './UiToast.vue'
import { useToast } from '@/composables/useToast'

// Mounted once by App.vue. Keeping the provider, viewport and portal here
// guarantees a single live region and a single F8 hotkey registration instead
// of one per panel that can raise feedback.
const { toast, dismiss } = useToast()
</script>

<template>
  <ToastProvider label="通知">
    <ToastPortal>
      <ToastViewport
        label="通知 ({hotkey})"
        class="fixed right-5 bottom-5 z-70 w-[calc(100vw_-_2.5rem)] max-w-96 list-none p-0 outline-none"
      >
        <UiToast
          v-if="toast"
          :key="toast.id"
          :message="toast.message"
          :tone="toast.tone"
          @dismiss="dismiss(toast.id)"
        />
      </ToastViewport>
    </ToastPortal>
  </ToastProvider>
</template>
