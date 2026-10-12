<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import { RouterLink } from 'vue-router'
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    to?: RouteLocationRaw
    href?: string
    target?: string
    rel?: string
    type?: 'button' | 'submit' | 'reset'
    label: string
    title?: string
    disabled?: boolean
    variant?: 'ghost' | 'danger'
  }>(),
  {
    to: undefined,
    href: undefined,
    target: undefined,
    rel: undefined,
    type: 'button',
    title: undefined,
    disabled: false,
    variant: 'ghost',
  },
)

const variantClass = computed(() =>
  props.variant === 'danger'
    ? 'text-soft-danger-fg hover:bg-soft-danger-bg active:brightness-95'
    : 'text-text-secondary hover:bg-surface-muted hover:text-text-primary active:bg-surface-solid-hover',
)

const classes = computed(() => [
  'inline-flex size-11 shrink-0 items-center justify-center rounded-[10px] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring disabled:cursor-not-allowed disabled:opacity-50',
  variantClass.value,
])

function preventDisabledLink(event: MouseEvent) {
  if (props.disabled) event.preventDefault()
}
</script>

<template>
  <RouterLink
    v-if="to"
    :to="to"
    :class="classes"
    :aria-label="label"
    :aria-disabled="disabled || undefined"
    :tabindex="disabled ? -1 : undefined"
    :title="title"
    @click="preventDisabledLink"
  >
    <slot />
  </RouterLink>
  <a
    v-else-if="href"
    :href="href"
    :target="target"
    :rel="rel"
    :class="classes"
    :aria-label="label"
    :aria-disabled="disabled || undefined"
    :tabindex="disabled ? -1 : undefined"
    :title="title"
    @click="preventDisabledLink"
  >
    <slot />
  </a>
  <button
    v-else
    :type="type"
    :disabled="disabled"
    :class="classes"
    :aria-label="label"
    :title="title"
  >
    <slot />
  </button>
</template>
