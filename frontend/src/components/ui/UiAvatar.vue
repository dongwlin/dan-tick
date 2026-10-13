<script setup lang="ts">
import { AvatarFallback, AvatarImage, AvatarRoot } from 'reka-ui'
import { UserRound } from 'lucide-vue-next'

defineProps<{ url?: string }>()
</script>

<template>
  <AvatarRoot
    class="inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full bg-(--accent-panel-alt) text-accent-text"
    aria-hidden="true"
  >
    <AvatarImage
      v-if="url"
      :src="url"
      alt=""
      referrerpolicy="no-referrer"
      class="size-full object-cover"
    />
    <AvatarFallback class="flex size-full items-center justify-center">
      <UserRound class="size-1/2" :stroke-width="1.8" />
    </AvatarFallback>
  </AvatarRoot>
</template>
