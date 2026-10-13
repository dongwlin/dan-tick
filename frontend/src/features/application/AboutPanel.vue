<script setup lang="ts">
import { computed } from 'vue'
import titlebarWordmark from '@/assets/branding/titlebar-wordmark.png'
import UiPanel from '@/components/ui/UiPanel.vue'
import UiAlert from '@/components/ui/UiAlert.vue'

const props = withDefaults(
  defineProps<{
    version?: string
    buildTime?: string
    commit?: string
    metadataUnavailable?: boolean
  }>(),
  { version: 'dev', buildTime: undefined, commit: undefined, metadataUnavailable: false },
)

const buildTimeLabel = computed(() => {
  if (!props.buildTime) return '未提供'
  const date = new Date(props.buildTime)
  if (Number.isNaN(date.getTime())) return '未提供'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    timeZoneName: 'short',
  }).format(date)
})

const shortCommit = computed(() => props.commit?.trim().slice(0, 8) || '未提供')
</script>

<template>
  <UiPanel aria-labelledby="about-heading">
    <h2 id="about-heading" class="text-base font-semibold">关于</h2>
    <div class="flex flex-col items-center px-2 pt-7 pb-5 text-center">
      <img
        :src="titlebarWordmark"
        alt="dan-tick"
        width="240"
        height="80"
        class="h-auto w-60 max-w-full object-contain"
        draggable="false"
      />
      <dl class="mt-5 grid w-fit max-w-full grid-cols-2 items-baseline gap-2 text-left text-sm">
        <dt class="text-right text-text-secondary">版本号</dt>
        <dd class="font-medium break-all text-text-primary tabular-nums">{{ version }}</dd>
        <dt class="text-right text-text-secondary">构建时间</dt>
        <dd class="text-text-primary tabular-nums">{{ buildTimeLabel }}</dd>
        <dt class="text-right text-text-secondary">Git Commit</dt>
        <dd class="font-mono text-text-primary tabular-nums">{{ shortCommit }}</dd>
      </dl>
    </div>
    <UiAlert v-if="metadataUnavailable" tone="warning">无法读取构建信息。</UiAlert>
  </UiPanel>
</template>
