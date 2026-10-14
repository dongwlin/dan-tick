<script setup lang="ts">
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { RadioGroupIndicator, RadioGroupItem, RadioGroupRoot } from 'reka-ui'
import { Check } from 'lucide-vue-next'
import { shallowRef, watch } from 'vue'
import { useToast } from '@/composables/useToast'
import UiPanel from '@/components/ui/UiPanel.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
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
const { notify, dismiss } = useToast()
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
    notify('保存窗口偏好失败，设置未更改。', 'error')
  },
})

function onChange(value: unknown) {
  if (value === 'hide' || value === 'quit') {
    dismiss()
    selected.value = value
    update.mutate(value)
  }
}
</script>

<template>
  <UiPanel aria-labelledby="close-behavior-heading">
    <h2 id="close-behavior-heading" class="mb-2 text-base font-semibold">窗口与后台运行</h2>
    <RadioGroupRoot
      :model-value="selected"
      :disabled="isPending || update.isPending.value"
      :aria-busy="isPending || update.isPending.value"
      orientation="vertical"
      aria-label="关闭窗口行为"
      class="flex flex-col gap-3"
      @update:model-value="onChange"
    >
      <RadioGroupItem
        value="hide"
        class="group flex min-h-11 items-center gap-3 rounded-[10px] border border-border-strong bg-surface-solid px-4 py-2.5 text-left text-text-primary hover:bg-surface-solid-hover data-[state=checked]:border-soft-accent-border data-[state=checked]:bg-soft-accent-bg disabled:cursor-not-allowed"
      >
        <span
          class="flex size-4 items-center justify-center rounded-full border border-border-strong group-data-[disabled]:opacity-60 group-data-[state=checked]:border-accent-solid"
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
        class="group flex min-h-11 items-center gap-3 rounded-[10px] border border-border-strong bg-surface-solid px-4 py-2.5 text-left text-text-primary hover:bg-surface-solid-hover data-[state=checked]:border-soft-accent-border data-[state=checked]:bg-soft-accent-bg disabled:cursor-not-allowed"
      >
        <span
          class="flex size-4 items-center justify-center rounded-full border border-border-strong group-data-[disabled]:opacity-60 group-data-[state=checked]:border-accent-solid"
          aria-hidden="true"
        >
          <RadioGroupIndicator class="flex size-3 items-center justify-center text-accent-text">
            <Check class="size-3" :stroke-width="2.5" aria-hidden="true" />
          </RadioGroupIndicator>
        </span>
        <span>关闭时退出应用，停止后台调度</span>
      </RadioGroupItem>
    </RadioGroupRoot>
    <UiAlert v-if="isError" tone="warning" class="mt-3">
      读取窗口偏好失败。
      <button type="button" class="underline" @click="refetch()">重试</button>
    </UiAlert>
  </UiPanel>
</template>
