<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import { RouterLink } from 'vue-router'
import { computed } from 'vue'
import { buttonClass, type ButtonSize, type ButtonVariant } from './buttonStyles'

const props = withDefaults(
  defineProps<{
    to?: RouteLocationRaw
    disabled?: boolean
    loading?: boolean
    variant?: ButtonVariant
    size?: ButtonSize
  }>(),
  { to: undefined, disabled: false, loading: false, variant: 'secondary', size: 'large' },
)

const classes = computed(() => buttonClass(props.variant, props.size))

function preventDisabledLink(event: MouseEvent) {
  if (props.disabled || props.loading) event.preventDefault()
}
</script>

<template>
  <RouterLink
    v-if="to"
    :to="to"
    :class="classes"
    :aria-disabled="disabled || loading || undefined"
    :aria-busy="loading || undefined"
    :tabindex="disabled || loading ? -1 : undefined"
    @click="preventDisabledLink"
  >
    <slot />
  </RouterLink>
  <button
    v-else
    type="button"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
    :class="classes"
  >
    <slot />
  </button>
</template>
