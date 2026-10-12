<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { CheckCircle2, CircleAlert, X } from 'lucide-vue-next'

const props = defineProps<{
  message: string
  tone: 'success' | 'error'
}>()
const emit = defineEmits<{ dismiss: [] }>()

let dismissTimer: ReturnType<typeof setTimeout> | undefined

onMounted(() => {
  dismissTimer = setTimeout(() => emit('dismiss'), props.tone === 'error' ? 8000 : 5000)
})

onUnmounted(() => clearTimeout(dismissTimer))
</script>

<template>
  <Teleport to="body">
    <div
      :role="tone === 'error' ? 'alert' : 'status'"
      :aria-label="tone === 'error' ? '操作失败通知' : '操作成功通知'"
      class="fixed right-5 bottom-5 z-70 flex w-[calc(100vw_-_2.5rem)] max-w-96 items-start gap-3 rounded-xl border bg-surface-solid px-4 py-3 text-sm text-text-primary shadow-(--shadow-overlay)"
      :class="tone === 'error' ? 'border-soft-danger-border' : 'border-soft-success-border'"
    >
      <CircleAlert
        v-if="tone === 'error'"
        class="mt-0.5 size-5 shrink-0 text-soft-danger-fg"
        aria-hidden="true"
      />
      <CheckCircle2 v-else class="mt-0.5 size-5 shrink-0 text-soft-success-fg" aria-hidden="true" />
      <span class="min-w-0 flex-1 py-0.5 wrap-anywhere">{{ message }}</span>
      <button
        type="button"
        aria-label="关闭通知"
        class="-m-2 flex size-10 shrink-0 items-center justify-center rounded-lg text-text-secondary hover:bg-surface-solid-hover"
        @click="emit('dismiss')"
      >
        <X class="size-4" aria-hidden="true" />
      </button>
    </div>
  </Teleport>
</template>
