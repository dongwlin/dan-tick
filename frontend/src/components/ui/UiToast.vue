<script setup lang="ts">
import { computed } from 'vue'
import { CheckCircle2, CircleAlert, Info, TriangleAlert, X } from 'lucide-vue-next'
import { ToastClose, ToastDescription, ToastRoot } from 'reka-ui'
import type { ToastTone } from '@/composables/useToast'

// One toast item. The provider, viewport and portal live in `UiToastHost`,
// which App.vue mounts once for the whole application.
const props = defineProps<{ message: string; tone: ToastTone }>()
const emit = defineEmits<{ dismiss: [] }>()

const roles = { success: 'status', info: 'status', warning: 'alert', error: 'alert' } as const
const labels = {
  success: '操作成功通知',
  info: '操作提示通知',
  warning: '操作警告通知',
  error: '操作失败通知',
} as const

const role = computed(() => roles[props.tone])
const label = computed(() => labels[props.tone])
const duration = computed(() => (props.tone === 'error' || props.tone === 'warning' ? 8000 : 5000))

function updateOpen(open: boolean) {
  if (!open) emit('dismiss')
}
</script>

<template>
  <ToastRoot
    :open="true"
    :duration="duration"
    :role="role"
    :aria-label="label"
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
    <CheckCircle2 v-else class="mt-0.5 size-5 shrink-0 text-soft-success-fg" aria-hidden="true" />
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
</template>
