<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Radio } from 'lucide-vue-next'
import { GetRoomCover } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'

const props = defineProps<{
  roomId: number
  platformRoomId: string
  index: number
}>()

const coverColors = ['bg-(--accent-panel)', 'bg-surface-muted', 'bg-(--accent-panel-alt)'] as const
const imageLoaded = shallowRef(false)
const imageFailed = shallowRef(false)
const { data: coverURL } = useQuery({
  queryKey: computed(() => ['room-cover', props.roomId]),
  queryFn: () => GetRoomCover(props.roomId),
  staleTime: 60 * 60 * 1000,
})

watch(coverURL, () => {
  imageLoaded.value = false
  imageFailed.value = false
})
</script>

<template>
  <div
    class="relative flex h-[134px] items-end justify-end overflow-hidden border-b border-border-hairline px-[19px] py-4"
    :class="coverColors[index % coverColors.length]"
  >
    <template v-if="!imageLoaded || imageFailed">
      <span
        class="absolute top-1/2 left-1/2 grid size-[66px] -translate-x-1/2 -translate-y-1/2 place-items-center rounded-full border border-soft-accent-border text-accent-text"
        aria-hidden="true"
      >
        <Radio class="size-[26px]" />
      </span>
      <span class="relative text-[11px] text-text-secondary tabular-nums">
        直播间 · {{ platformRoomId }}
      </span>
    </template>
    <img
      v-if="coverURL && !imageFailed"
      :src="coverURL"
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
