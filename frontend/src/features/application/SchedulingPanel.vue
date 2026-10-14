<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { fieldClass } from '@/components/ui/uiStyles'
import UiPanel from '@/components/ui/UiPanel.vue'
import UiAsyncSection from '@/components/ui/UiAsyncSection.vue'
import { useToast } from '@/composables/useToast'
import { timezonePickerOptions } from '@/lib/timezones'
import { usePreferences, useSetDefaultTimezone, useSetSchedulingEnabled } from './queries'

const preferences = usePreferences()
const setTimezone = useSetDefaultTimezone()
const setScheduling = useSetSchedulingEnabled()
const { notify, dismiss } = useToast()

// The select commits as soon as a zone is picked, so its own value is held
// locally until the backend or a re-read reports what is stored. The stored
// zone is authoritative: never overwrite it with the value of a write whose
// response failed.
const localTimezone = ref('')
const storedTimezone = computed(() => preferences.data.value?.defaultTimezone ?? '')
watch(storedTimezone, (value) => (localTimezone.value = value), { immediate: true })
watch(
  () => setTimezone.isPending.value,
  (pending) => {
    if (!pending) localTimezone.value = storedTimezone.value
  },
)
const options = computed(() => timezonePickerOptions(storedTimezone.value))

async function changeTimezone(value: string) {
  if (!value || value === storedTimezone.value) return
  notify('正在保存默认时区…', 'info')
  try {
    await setTimezone.mutateAsync(value)
    notify('默认时区已保存，仅应用于新建任务。', 'success')
  } catch {
    notify('默认时区保存失败，请重新选择时区后重试。', 'error')
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
          class="group flex min-h-15 w-full items-center justify-between gap-5 py-3 text-left focus-visible:outline-hidden"
          :class="{ 'cursor-pointer': setScheduling.isPending.value }"
          aria-label="启用自动调度"
          :model-value="preferences.data.value?.schedulingEnabled ?? false"
          :disabled="setScheduling.isPending.value"
          @update:model-value="toggleScheduling"
        >
          <strong class="text-sm">启用自动调度</strong>
          <span
            aria-hidden="true"
            class="flex h-[19px] w-[31px] shrink-0 items-center rounded-full border border-border-strong bg-surface-muted px-[2px] group-data-[state=checked]:border-accent-solid group-data-[state=checked]:bg-accent-solid group-hover:bg-surface-solid-hover group-data-[state=checked]:group-hover:bg-accent-solid group-data-[state=checked]:group-hover:brightness-95 group-active:brightness-90 group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-focus-ring motion-safe:transition-colors motion-safe:duration-200"
          >
            <SwitchThumb
              class="pointer-events-none block size-[13px] rounded-full border border-border-hairline bg-text-tertiary data-[state=checked]:translate-x-3 data-[state=checked]:bg-on-accent motion-safe:transition-[translate,background-color] motion-safe:duration-200"
            />
          </span>
        </SwitchRoot>
        <div class="flex min-h-20 flex-wrap items-center justify-between gap-x-5 gap-y-3 py-4">
          <label for="default-timezone" class="text-sm font-medium">
            <strong>默认时区</strong>
          </label>
          <select
            id="default-timezone"
            v-model="localTimezone"
            :class="[fieldClass, 'w-auto min-w-56 max-w-72']"
            :disabled="setTimezone.isPending.value"
            :aria-busy="setTimezone.isPending.value"
            @change="changeTimezone(localTimezone)"
          >
            <option v-for="option in options" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </div>
      </div>
    </UiAsyncSection>
  </UiPanel>
</template>
