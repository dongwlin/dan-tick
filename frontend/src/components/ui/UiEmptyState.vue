<script setup lang="ts">
import type { Component } from 'vue'
import { RouterLink, type RouteLocationRaw } from 'vue-router'
import { secondaryButton } from './buttonStyles'

withDefaults(
  defineProps<{
    icon?: Component
    title?: string
    description?: string
    actionLabel?: string
    actionTo?: RouteLocationRaw
    /** Use the available height without imposing a 160px minimum. */
    fill?: boolean
  }>(),
  {
    icon: undefined,
    title: undefined,
    description: undefined,
    actionLabel: undefined,
    actionTo: undefined,
    fill: true,
  },
)

const emit = defineEmits<{ action: [] }>()
</script>

<template>
  <div
    :class="[
      'flex flex-1 flex-col items-center justify-center gap-2 text-center',
      fill ? 'min-h-40 p-8' : 'min-h-0 p-5',
    ]"
  >
    <slot name="icon">
      <component :is="icon" v-if="icon" class="size-7 text-text-tertiary" aria-hidden="true" />
    </slot>
    <p v-if="title" class="wrap-anywhere font-medium text-text-primary">{{ title }}</p>
    <p v-if="description" class="text-sm text-text-secondary">{{ description }}</p>
    <slot />
    <RouterLink v-if="actionTo && actionLabel" :to="actionTo" :class="[secondaryButton, 'mt-1']">{{
      actionLabel
    }}</RouterLink>
    <button
      v-else-if="actionLabel"
      type="button"
      :class="[secondaryButton, 'mt-1']"
      @click="emit('action')"
    >
      {{ actionLabel }}
    </button>
  </div>
</template>
