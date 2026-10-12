<script setup lang="ts">
import { CheckCircle2, CircleAlert, Info, TriangleAlert, X } from 'lucide-vue-next'
import {
  ToastClose,
  ToastDescription,
  ToastPortal,
  ToastProvider,
  ToastRoot,
  ToastViewport,
} from 'reka-ui'

defineProps<{
  message: string
  tone: 'success' | 'error' | 'warning' | 'info'
}>()
const emit = defineEmits<{ dismiss: [] }>()

function updateOpen(open: boolean) {
  if (!open) emit('dismiss')
}
</script>

<template>
  <Teleport to="body">
    <ToastProvider label="通知">
      <ToastRoot
        :open="true"
        :duration="tone === 'error' || tone === 'warning' ? 8000 : 5000"
        :role="tone === 'error' || tone === 'warning' ? 'alert' : 'status'"
        :aria-label="
          tone === 'error'
            ? '操作失败通知'
            : tone === 'warning'
              ? '操作警告通知'
              : tone === 'info'
                ? '操作提示通知'
                : '操作成功通知'
        "
        class="flex w-full items-start gap-3 rounded-xl border bg-surface-solid px-4 py-3 text-sm text-text-primary shadow-(--shadow-overlay)"
        :class="{
          'border-soft-danger-border': tone === 'error',
          'border-soft-warning-border': tone === 'warning',
          'border-soft-accent-border': tone === 'info',
          'border-soft-success-border': tone === 'success',
        }"
        @update:open="updateOpen"
      >
        <CircleAlert
          v-if="tone === 'error'"
          class="mt-0.5 size-5 shrink-0 text-soft-danger-fg"
          aria-hidden="true"
        />
        <TriangleAlert
          v-else-if="tone === 'warning'"
          class="mt-0.5 size-5 shrink-0 text-soft-warning-fg"
          aria-hidden="true"
        />
        <Info
          v-else-if="tone === 'info'"
          class="mt-0.5 size-5 shrink-0 text-accent-text"
          aria-hidden="true"
        />
        <CheckCircle2
          v-else
          class="mt-0.5 size-5 shrink-0 text-soft-success-fg"
          aria-hidden="true"
        />
        <ToastDescription as="span" class="min-w-0 flex-1 py-0.5 wrap-anywhere">
          {{ message }}
        </ToastDescription>
        <ToastClose as-child>
          <button
            type="button"
            aria-label="关闭通知"
            class="-m-2 flex size-11 shrink-0 items-center justify-center rounded-lg text-text-secondary hover:bg-surface-solid-hover active:bg-surface-muted"
          >
            <X class="size-4" aria-hidden="true" />
          </button>
        </ToastClose>
      </ToastRoot>
      <ToastPortal>
        <ToastViewport
          label="通知 ({hotkey})"
          class="fixed right-5 bottom-5 z-70 w-[calc(100vw_-_2.5rem)] max-w-96 list-none p-0 outline-none"
        />
      </ToastPortal>
    </ToastProvider>
  </Teleport>
</template>
