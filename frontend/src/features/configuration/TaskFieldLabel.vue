<script setup lang="ts">
defineProps<{ label: string; error?: string; errorId: string }>()
</script>

<template>
  <span class="flex min-w-0 items-center justify-between gap-2">
    <span class="shrink-0">{{ label }}</span>
    <span
      :id="errorId"
      :title="error || undefined"
      class="min-w-0 truncate text-right text-xs leading-5 text-soft-danger-fg"
      >{{ error }}</span
    >
  </span>
</template>
