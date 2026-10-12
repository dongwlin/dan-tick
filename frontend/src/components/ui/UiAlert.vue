<script setup lang="ts">
import { computed } from 'vue'
import { CircleAlert, CircleCheck, Info, TriangleAlert } from 'lucide-vue-next'

type AlertTone = 'accent' | 'success' | 'warning' | 'danger'

const props = withDefaults(
  defineProps<{
    tone?: AlertTone
    /** `soft` is the standard notice bar; `plain` is for inline supporting text. */
    variant?: 'soft' | 'plain'
    /** Override the live-region role; pass an empty string for static notices. */
    role?: string
    showIcon?: boolean
  }>(),
  {
    tone: 'warning',
    variant: 'soft',
    role: 'alert',
    showIcon: true,
  },
)

const icons = {
  accent: Info,
  success: CircleCheck,
  warning: TriangleAlert,
  danger: CircleAlert,
} as const

const softClasses: Record<AlertTone, string> = {
  accent: 'border-soft-accent-border bg-soft-accent-bg text-soft-accent-fg',
  success: 'border-soft-success-border bg-soft-success-bg text-soft-success-fg',
  warning: 'border-soft-warning-border bg-soft-warning-bg text-soft-warning-fg',
  danger: 'border-soft-danger-border bg-soft-danger-bg text-soft-danger-fg',
}

const plainClasses: Record<AlertTone, string> = {
  accent: 'text-soft-accent-fg',
  success: 'text-soft-success-fg',
  warning: 'text-soft-warning-fg',
  danger: 'text-soft-danger-fg',
}

const classes = computed(() =>
  props.variant === 'plain'
    ? ['flex items-start gap-2 text-sm', plainClasses[props.tone]]
    : ['flex items-start gap-2 rounded-[10px] border px-3 py-2 text-sm', softClasses[props.tone]],
)
</script>

<template>
  <div :role="role || undefined" :class="classes">
    <component
      :is="icons[tone]"
      v-if="showIcon"
      class="mt-0.5 size-4 shrink-0"
      aria-hidden="true"
    />
    <div class="min-w-0 flex-1">
      <slot />
    </div>
  </div>
</template>
