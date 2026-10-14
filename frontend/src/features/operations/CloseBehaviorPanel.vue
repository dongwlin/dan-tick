<script setup lang="ts">
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { useToast } from '@/composables/useToast'
import UiPanel from '@/components/ui/UiPanel.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { fieldClass } from '@/components/ui/uiStyles'
import {
  GetCloseBehavior,
  SetCloseBehavior,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice'

// The stored values are the backend contract; the labels are the wording this
// row shows for them.
const options: { value: string; label: string }[] = [
  { value: 'hide', label: '最小化到系统托盘' },
  { value: 'quit', label: '退出应用程序' },
]

const queryClient = useQueryClient()
const queryKey = ['desktop', 'close-behavior'] as const
const {
  data: behavior,
  isFetching,
  isError,
  refetch,
} = useQuery({
  queryKey,
  queryFn: GetCloseBehavior,
})
const { notify, dismiss } = useToast()
const update = useMutation({
  mutationFn: (value: string) => SetCloseBehavior(value),
  // Optimistic: the chosen option shows while the write runs, and the reread
  // afterwards reports what the backend really holds — a write can succeed
  // before its response fails.
  onMutate: (value) => queryClient.setQueryData(queryKey, value),
  onSettled: () => queryClient.invalidateQueries({ queryKey }),
})

function onChange(value: string) {
  if ((value !== 'hide' && value !== 'quit') || value === behavior.value) return
  dismiss()
  update.mutate(value, {
    onError: () => notify('更改窗口行为未成功确认，请确认当前选择后重试。', 'error'),
  })
}
</script>

<template>
  <UiPanel aria-labelledby="close-behavior-heading">
    <h2 id="close-behavior-heading" class="mb-2 text-base font-semibold">窗口行为</h2>
    <div class="flex min-h-15 flex-wrap items-start justify-between gap-x-5 gap-y-3 py-3">
      <label for="close-behavior" class="text-sm font-medium"><strong>关闭窗口时</strong></label>
      <UiSelect
        id="close-behavior"
        :model-value="behavior"
        :options="options"
        :class="[fieldClass, 'w-auto min-w-48 max-w-72 disabled:cursor-pointer']"
        :disabled="isFetching || update.isPending.value"
        :aria-busy="isFetching || update.isPending.value"
        @update:model-value="onChange"
      />
    </div>
    <UiAlert v-if="isError" tone="warning" class="mt-3">
      读取窗口偏好失败。
      <button type="button" class="underline" @click="refetch()">重试</button>
    </UiAlert>
  </UiPanel>
</template>
