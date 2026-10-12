<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { UserRound } from 'lucide-vue-next'

const props = defineProps<{ url?: string }>()
const failedUrl = shallowRef('')
const imageUrl = computed(() => (props.url && props.url !== failedUrl.value ? props.url : ''))
</script>

<template>
  <span
    class="inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full bg-(--accent-panel-alt) text-accent-text"
    aria-hidden="true"
  >
    <img
      v-if="imageUrl"
      :src="imageUrl"
      alt=""
      referrerpolicy="no-referrer"
      class="size-full object-cover"
      @error="failedUrl = imageUrl"
    />
    <UserRound v-else class="size-1/2" :stroke-width="1.8" />
  </span>
</template>
