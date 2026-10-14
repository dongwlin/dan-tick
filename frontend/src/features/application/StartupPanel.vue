<script setup lang="ts">
import { computed, watch } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import UiPanel from '@/components/ui/UiPanel.vue'
import { useToast } from '@/composables/useToast'
import {
  useAutostartStatus,
  useSetAutostartEnabled,
  useSetStartupVisibility,
  useStartupVisibility,
} from './queries'

const status = useAutostartStatus()
const setAutostart = useSetAutostartEnabled()
const visibility = useStartupVisibility()
const setVisibility = useSetStartupVisibility()
const busy = computed(
  () => status.isFetching.value || setAutostart.isPending.value || setVisibility.isPending.value,
)
const autostartDisabled = computed(
  () => busy.value || status.isError.value || !status.data.value?.supported,
)
const visibilityDisabled = computed(
  () => busy.value || visibility.isError.value || visibility.isPending.value,
)
const hideOnStartup = computed(() => visibility.data.value === 'hide')
const { notify } = useToast()

// Every message about these two settings lives in the shared toast slot: the
// card renders only the switches, so a read, a write or a system state can
// never add or remove a row and shift the layout around it. Re-entering the
// settings page is what reads a failed state again.
watch(
  () => status.error.value,
  (error) => {
    if (error) notify('无法读取开机自启状态，请重新进入设置页后再试。', 'error')
  },
  { immediate: true },
)
watch(
  () => visibility.error.value,
  (error) => {
    if (error) notify('无法读取静默启动设置，请重新进入设置页后再试。', 'error')
  },
  { immediate: true },
)
watch(
  () => status.data.value?.supported,
  (supported) => {
    if (supported === false) notify('当前系统不支持开机自启。', 'warning')
  },
  { immediate: true },
)
watch(
  () => status.data.value?.needsRepair,
  (needsRepair) => {
    if (needsRepair) notify('已有启动项需要修复，重新开启即可更新。', 'warning')
  },
  { immediate: true },
)

async function changeAutostart(enabled: boolean) {
  if (autostartDisabled.value) return
  notify('正在更新开机自启状态…', 'info')
  try {
    await setAutostart.mutateAsync(enabled)
    notify(enabled ? '已开启开机自启。' : '已关闭开机自启。', 'success')
  } catch {
    notify('更改开机自启未成功确认，请确认当前状态后重试。', 'error')
  }
}

async function changeStartupVisibility(enabled: boolean) {
  if (visibilityDisabled.value) return
  notify('正在更新静默启动设置…', 'info')
  try {
    await setVisibility.mutateAsync(enabled ? 'hide' : 'show')
    notify(
      enabled ? '已开启静默启动，下次启动生效。' : '已关闭静默启动，下次启动显示主窗口。',
      'success',
    )
  } catch {
    notify('更改静默启动未成功确认，请确认当前状态后重试。', 'error')
  }
}
</script>

<template>
  <UiPanel aria-labelledby="startup-heading">
    <h2 id="startup-heading" class="text-base font-semibold">启动设置</h2>
    <div class="divide-y divide-border-hairline">
      <SwitchRoot
        id="autostart-switch"
        class="group flex min-h-20 w-full items-center justify-between gap-5 py-3 text-left focus-visible:outline-hidden disabled:cursor-not-allowed"
        aria-label="开机自启"
        aria-describedby="autostart-description"
        :model-value="(status.data.value?.enabled && !status.data.value.needsRepair) ?? false"
        :disabled="autostartDisabled"
        :aria-busy="setAutostart.isPending.value"
        @update:model-value="changeAutostart"
      >
        <span class="flex min-w-0 flex-col gap-1">
          <strong class="text-sm">开机自启</strong>
          <span id="autostart-description" class="text-xs text-text-tertiary">
            登录桌面会话后自动启动 dan-tick
          </span>
        </span>
        <span
          aria-hidden="true"
          class="flex h-[19px] w-[31px] shrink-0 items-center rounded-full border border-border-strong bg-surface-muted px-[2px] group-data-[state=checked]:border-accent-solid group-data-[state=checked]:bg-accent-solid group-hover:bg-surface-solid-hover group-data-[state=checked]:group-hover:bg-accent-solid group-data-[state=checked]:group-hover:brightness-95 group-active:brightness-90 group-data-[disabled]:opacity-60 group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-focus-ring motion-safe:transition-colors motion-safe:duration-200"
        >
          <SwitchThumb
            class="pointer-events-none block size-[13px] rounded-full border border-border-hairline bg-text-tertiary data-[state=checked]:translate-x-3 data-[state=checked]:bg-on-accent motion-safe:transition-[translate,background-color] motion-safe:duration-200"
          />
        </span>
      </SwitchRoot>
      <SwitchRoot
        id="startup-visibility-switch"
        class="group flex min-h-20 w-full items-center justify-between gap-5 py-3 text-left focus-visible:outline-hidden disabled:cursor-not-allowed"
        aria-label="静默启动"
        aria-describedby="startup-visibility-description"
        :model-value="hideOnStartup"
        :disabled="visibilityDisabled"
        :aria-busy="setVisibility.isPending.value"
        @update:model-value="changeStartupVisibility"
      >
        <span class="flex min-w-0 flex-col gap-1">
          <strong class="text-sm">静默启动</strong>
          <span id="startup-visibility-description" class="text-xs text-text-tertiary">
            启动后不显示主窗口，仅保留系统托盘入口
          </span>
        </span>
        <span
          aria-hidden="true"
          class="flex h-[19px] w-[31px] shrink-0 items-center rounded-full border border-border-strong bg-surface-muted px-[2px] group-data-[state=checked]:border-accent-solid group-data-[state=checked]:bg-accent-solid group-hover:bg-surface-solid-hover group-data-[state=checked]:group-hover:bg-accent-solid group-data-[state=checked]:group-hover:brightness-95 group-active:brightness-90 group-data-[disabled]:opacity-60 group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-focus-ring motion-safe:transition-colors motion-safe:duration-200"
        >
          <SwitchThumb
            class="pointer-events-none block size-[13px] rounded-full border border-border-hairline bg-text-tertiary data-[state=checked]:translate-x-3 data-[state=checked]:bg-on-accent motion-safe:transition-[translate,background-color] motion-safe:duration-200"
          />
        </span>
      </SwitchRoot>
    </div>
  </UiPanel>
</template>
