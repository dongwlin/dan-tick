<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    /** Element used for the panel surface. */
    as?: string
    /** Acrylic panels sit on canvas content; solid panels group content inside them. */
    variant?: 'acrylic' | 'solid'
    /** Remove the default 20px padding when the panel manages its own layout. */
    padded?: boolean
  }>(),
  {
    as: 'section',
    variant: 'acrylic',
    padded: true,
  },
)

const panelClass = computed(() => [
  props.variant === 'solid'
    ? 'rounded-[14px] border border-border-hairline bg-surface-solid'
    : 'acrylic rounded-[14px]',
  props.padded ? 'p-5' : '',
])
</script>

<template>
  <component :is="as" :class="panelClass">
    <slot />
  </component>
</template>
