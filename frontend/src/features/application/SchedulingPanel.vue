<script setup lang="ts">
import { shallowRef, watch } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { fieldClass } from '@/components/ui/uiStyles'
import UiPanel from '@/components/ui/UiPanel.vue'
import UiAsyncSection from '@/components/ui/UiAsyncSection.vue'
import { secondaryButton } from '@/components/ui/buttonStyles'
import { useToast } from '@/composables/useToast'
import { usePreferences, useSetDefaultTimezone, useSetSchedulingEnabled } from './queries'

const preferences = usePreferences()
const setTimezone = useSetDefaultTimezone()
const setScheduling = useSetSchedulingEnabled()
const timezone = shallowRef('')
const { notify, dismiss } = useToast()

watch(
  () => preferences.data.value?.defaultTimezone,
  (value) => {
    if (value && !setTimezone.isPending.value) timezone.value = value
  },
  { immediate: true },
)

async function saveTimezone() {
  dismiss()
  try {
    await setTimezone.mutateAsync(timezone.value.trim())
    notify('默认时区已保存，仅应用于新建任务。', 'success')
  } catch {
    notify('默认时区保存失败，请检查时区名称后重试。', 'error')
  }
}

async function toggleScheduling(enabled: boolean) {
  dismiss()
  try {
    await setScheduling.mutateAsync(enabled)
    notify(
      enabled ? '自动调度已恢复。' : '已暂停新的自动调度，当前运行中的任务不受影响。',
      'success',
    )
  } catch {
    notify('更改调度状态未成功确认，请重新读取当前状态。', 'error')
  }
}
</script>

<template>
  <UiPanel aria-label="调度设置">
    <h2 class="mb-2 text-base font-semibold">调度设置</h2>
    <UiAsyncSection
      :pending="preferences.isPending.value"
      :error="preferences.error.value"
      loading-text="正在读取调度设置…"
      retry-label="重新读取"
      dense
      @retry="preferences.refetch()"
    >
      <div class="divide-y divide-border-hairline">
        <SwitchRoot
          id="scheduling-switch"
          class="group flex min-h-20 w-full items-center justify-between gap-5 py-3 text-left focus-visible:outline-hidden"
          :class="{ 'cursor-pointer': setScheduling.isPending.value }"
          aria-label="启用自动调度"
          aria-describedby="scheduling-description"
          :model-value="preferences.data.value?.schedulingEnabled ?? false"
          :disabled="setScheduling.isPending.value"
          @update:model-value="toggleScheduling"
        >
          <span class="flex min-w-0 flex-col gap-1">
            <strong class="text-sm">启用自动调度</strong>
            <span id="scheduling-description" class="text-xs text-text-tertiary">
              暂停后不会触发新的计划，手动执行仍可使用
            </span>
          </span>
          <span
            aria-hidden="true"
            class="flex h-[19px] w-[31px] shrink-0 items-center rounded-full border border-border-strong bg-surface-muted px-[2px] group-data-[state=checked]:border-accent-solid group-data-[state=checked]:bg-accent-solid group-hover:bg-surface-solid-hover group-data-[state=checked]:group-hover:bg-accent-solid group-data-[state=checked]:group-hover:brightness-95 group-active:brightness-90 group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-focus-ring motion-safe:transition-colors motion-safe:duration-200"
          >
            <SwitchThumb
              class="pointer-events-none block size-[13px] rounded-full border border-border-hairline bg-text-tertiary data-[state=checked]:translate-x-3 data-[state=checked]:bg-on-accent motion-safe:transition-[translate,background-color] motion-safe:duration-200"
            />
          </span>
        </SwitchRoot>
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
            :class="[secondaryButton, 'min-w-32']"
            :disabled="setTimezone.isPending.value || !timezone.trim()"
            :aria-busy="setTimezone.isPending.value"
          >
            {{ setTimezone.isPending.value ? '正在保存…' : '保存默认时区' }}
          </button>
        </form>
      </div>
    </UiAsyncSection>
  </UiPanel>
</template>
