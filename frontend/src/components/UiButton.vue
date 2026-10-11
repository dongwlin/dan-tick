<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import { RouterLink } from 'vue-router'

withDefaults(
  defineProps<{
    to?: RouteLocationRaw
    disabled?: boolean
  }>(),
  { to: undefined, disabled: false },
)

const buttonClass =
  'ui-button inline-flex min-h-11 items-center justify-center gap-2 rounded-[10px] border border-border-strong bg-surface-solid px-4 py-2.5 font-medium text-text-primary transition-colors hover:bg-surface-solid-hover active:bg-surface-muted disabled:cursor-not-allowed disabled:opacity-50'
</script>

<template>
  <RouterLink v-if="to" :to="to" :class="buttonClass">
    <slot />
  </RouterLink>
  <button v-else type="button" :disabled="disabled" :class="buttonClass">
    <slot />
  </button>
</template>
