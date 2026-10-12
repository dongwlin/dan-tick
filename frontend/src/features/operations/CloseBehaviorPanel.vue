<script setup lang="ts">
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { RadioGroupIndicator, RadioGroupItem, RadioGroupRoot } from 'reka-ui'
import { Check } from 'lucide-vue-next'
import { shallowRef, watch } from 'vue'
import {
  GetCloseBehavior,
  SetCloseBehavior,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice'

const queryClient = useQueryClient()
const queryKey = ['desktop', 'close-behavior'] as const
const {
  data: behavior,
  isPending,
  isError,
  refetch,
} = useQuery({
  queryKey,
  queryFn: GetCloseBehavior,
})
const selected = shallowRef<string | undefined>()
watch(
  behavior,
  (value) => {
    selected.value = value
  },
  { immediate: true },
)
const update = useMutation({
  mutationFn: (value: string) => SetCloseBehavior(value),
  onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  onError: () => {
    selected.value = behavior.value
  },
})

function onChange(value: unknown) {
  if (value === 'hide' || value === 'quit') {
    selected.value = value
    update.mutate(value)
  }
}
</script>

<template>
  <section
    class="rounded-2xl border border-border-hairline bg-surface-solid p-5"
    aria-labelledby="close-behavior-heading"
  >
    <h2 id="close-behavior-heading" class="mb-2 text-lg font-semibold">窗口与后台运行</h2>
    <p id="close-behavior-description" class="mb-4 text-sm text-text-tertiary">
      无系统托盘时，可再次启动 dan-tick 找回已隐藏的窗口；不会启动第二份调度器。
    </p>
    <RadioGroupRoot
      :model-value="selected"
      :disabled="isPending || update.isPending.value"
      orientation="vertical"
      aria-label="关闭窗口行为"
      aria-describedby="close-behavior-description"
      class="flex flex-col gap-3"
      @update:model-value="onChange"
    >
      <RadioGroupItem
        value="hide"
        class="group flex min-h-11 items-center gap-3 rounded-[10px] border border-border-strong bg-surface-solid px-4 py-2.5 text-left text-text-primary hover:bg-surface-solid-hover data-[state=checked]:border-soft-accent-border data-[state=checked]:bg-soft-accent-bg disabled:opacity-60"
      >
        <span
          class="flex size-4 items-center justify-center rounded-full border border-border-strong group-data-[state=checked]:border-accent-solid"
          aria-hidden="true"
        >
          <RadioGroupIndicator class="flex size-3 items-center justify-center text-accent-text">
            <Check class="size-3" :stroke-width="2.5" aria-hidden="true" />
          </RadioGroupIndicator>
        </span>
        <span>关闭时隐藏窗口，继续运行任务（默认）</span>
      </RadioGroupItem>
      <RadioGroupItem
        value="quit"
        class="group flex min-h-11 items-center gap-3 rounded-[10px] border border-border-strong bg-surface-solid px-4 py-2.5 text-left text-text-primary hover:bg-surface-solid-hover data-[state=checked]:border-soft-accent-border data-[state=checked]:bg-soft-accent-bg disabled:opacity-60"
      >
        <span
          class="flex size-4 items-center justify-center rounded-full border border-border-strong group-data-[state=checked]:border-accent-solid"
          aria-hidden="true"
        >
          <RadioGroupIndicator class="flex size-3 items-center justify-center text-accent-text">
            <Check class="size-3" :stroke-width="2.5" aria-hidden="true" />
          </RadioGroupIndicator>
        </span>
        <span>关闭时退出应用，停止后台调度</span>
      </RadioGroupItem>
    </RadioGroupRoot>
    <p v-if="isPending" role="status" class="mt-3 text-sm text-text-secondary">正在读取窗口偏好…</p>
    <p v-if="update.isPending.value" role="status" class="mt-3 text-sm text-text-secondary">
      正在保存…
    </p>
    <p
      v-if="isError || update.isError.value"
      role="alert"
      class="mt-3 text-sm text-soft-warning-fg"
    >
      {{ isError ? '读取窗口偏好失败。' : '保存窗口偏好失败，设置未更改。' }}
      <button v-if="isError" type="button" class="underline" @click="refetch()">重试</button>
    </p>
    <p class="mt-3 text-sm text-text-tertiary">
      标题栏的「退出应用」始终可以明确结束后台任务；存在已启用或运行中的任务时会询问确认。
    </p>
  </section>
</template>
