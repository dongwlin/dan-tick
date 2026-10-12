<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    description?: string
  }>(),
  { description: undefined },
)
</script>

<template>
  <header class="flex flex-wrap items-center justify-between gap-3">
    <div class="min-w-0">
      <h1 class="wrap-anywhere">{{ title }}</h1>
      <p v-if="description" class="mt-1 text-sm text-text-secondary">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="flex flex-wrap items-center justify-end gap-2">
      <slot name="actions" />
    </div>
  </header>
</template>
