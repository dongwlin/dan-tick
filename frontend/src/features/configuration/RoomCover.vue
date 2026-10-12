<script setup lang="ts">
import { shallowRef, watch } from 'vue'
import { Radio } from 'lucide-vue-next'

const props = defineProps<{
  coverUrl: string
  index: number
}>()

const coverColors = ['bg-(--accent-panel)', 'bg-surface-muted', 'bg-(--accent-panel-alt)'] as const
const imageLoaded = shallowRef(false)
const imageFailed = shallowRef(false)
watch(
  () => props.coverUrl,
  () => {
    imageLoaded.value = false
    imageFailed.value = false
  },
)
</script>

<template>
  <div
    class="relative h-[134px] overflow-hidden border-b border-border-hairline"
    :class="coverColors[index % coverColors.length]"
  >
    <template v-if="!imageLoaded || imageFailed">
      <span
        class="absolute top-1/2 left-1/2 grid size-[66px] -translate-x-1/2 -translate-y-1/2 place-items-center rounded-full border border-soft-accent-border text-accent-text"
        aria-hidden="true"
      >
        <Radio class="size-[26px]" />
      </span>
    </template>
    <img
      v-if="coverUrl && !imageFailed"
      :src="coverUrl"
      alt=""
      loading="lazy"
      referrerpolicy="no-referrer"
      class="absolute inset-0 size-full object-cover"
      :class="imageLoaded ? '' : 'opacity-0'"
      @load="imageLoaded = true"
      @error="imageFailed = true"
    />
  </div>
</template>
