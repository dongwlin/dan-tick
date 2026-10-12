<script setup lang="ts">
import { shallowRef, watch } from 'vue'
import { bindingError } from '@/features/configuration/queries'
import { fieldClass, secondaryButton } from '@/features/configuration/style'
import { usePreferences, useSetDefaultTimezone, useSetSchedulingEnabled } from './queries'

const preferences = usePreferences()
const setTimezone = useSetDefaultTimezone()
const setScheduling = useSetSchedulingEnabled()
const timezone = shallowRef('')
const feedback = shallowRef('')

watch(
  () => preferences.data.value?.defaultTimezone,
  (value) => {
    if (value && !setTimezone.isPending.value) timezone.value = value
  },
  { immediate: true },
)

async function saveTimezone() {
  feedback.value = ''
  try {
    await setTimezone.mutateAsync(timezone.value.trim())
    feedback.value = '默认时区已保存，仅应用于新建任务。'
  } catch {
    feedback.value = '默认时区保存失败，请检查时区名称后重试。'
  }
}

async function toggleScheduling(event: Event) {
  const target = event.target as HTMLInputElement
  const enabled = target.checked
  target.checked = preferences.data.value?.schedulingEnabled ?? !enabled
  feedback.value = ''
  try {
    await setScheduling.mutateAsync(enabled)
    feedback.value = enabled ? '自动调度已恢复。' : '已暂停新的自动调度，当前运行中的任务不受影响。'
  } catch {
    feedback.value = '更改调度状态未成功确认，请重新读取当前状态。'
  }
}
</script>

<template>
  <section class="acrylic rounded-[14px] p-5" aria-label="调度设置">
    <h2 class="mb-2 text-base font-semibold">调度设置</h2>
    <div v-if="preferences.isPending.value" role="status" class="py-4 text-sm text-text-secondary">
      正在读取调度设置…
    </div>
    <div
      v-else-if="preferences.isError.value"
      role="alert"
      class="py-4 text-sm text-soft-danger-fg"
    >
      {{ bindingError(preferences.error.value) }}
      <button type="button" class="ml-2 underline" @click="preferences.refetch()">重新读取</button>
    </div>
    <div v-else class="divide-y divide-border-hairline">
      <label for="scheduling-switch" class="flex min-h-20 items-center justify-between gap-5 py-3">
        <span class="flex flex-col gap-1">
          <strong class="text-sm">启用自动调度</strong>
          <span class="text-xs text-text-tertiary">暂停后不会触发新的计划，手动执行仍可使用</span>
        </span>
        <input
          id="scheduling-switch"
          type="checkbox"
          class="size-5 shrink-0 accent-accent-solid"
          :checked="preferences.data.value?.schedulingEnabled ?? false"
          :disabled="setScheduling.isPending.value"
          @change="toggleScheduling"
        />
      </label>
      <form
        class="flex flex-wrap items-end justify-between gap-4 py-4"
        @submit.prevent="saveTimezone"
      >
        <label for="default-timezone" class="flex min-w-44 flex-1 flex-col gap-1.5 text-sm">
          <strong>默认时区</strong>
          <span class="text-xs text-text-tertiary">使用 IANA 时区名称；只影响新建任务</span>
          <input
            id="default-timezone"
            v-model="timezone"
            :class="fieldClass"
            required
            maxlength="100"
            placeholder="Asia/Shanghai"
            autocomplete="off"
          />
        </label>
        <button
          type="submit"
          :class="secondaryButton"
          :disabled="setTimezone.isPending.value || !timezone.trim()"
        >
          {{ setTimezone.isPending.value ? '正在保存…' : '保存默认时区' }}
        </button>
      </form>
    </div>
    <p v-if="feedback" class="mt-2 text-sm text-text-secondary" role="status">{{ feedback }}</p>
  </section>
</template>
