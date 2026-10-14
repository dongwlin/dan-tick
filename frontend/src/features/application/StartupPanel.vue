<script setup lang="ts">
import { computed } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiAsyncSection from '@/components/ui/UiAsyncSection.vue'
import UiButton from '@/components/ui/UiButton.vue'
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
const { notify, dismiss } = useToast()

async function changeAutostart(enabled: boolean) {
  if (autostartDisabled.value) return
  dismiss()
  try {
    await setAutostart.mutateAsync(enabled)
    notify(enabled ? '已开启开机自启动。' : '已关闭开机自启动。', 'success')
  } catch {
    notify('更改开机自启动未成功确认，请查看当前状态或重新读取。', 'error')
  }
}

async function changeStartupVisibility(enabled: boolean) {
  if (visibilityDisabled.value) return
  dismiss()
  try {
    await setVisibility.mutateAsync(enabled ? 'hide' : 'show')
    notify(
      enabled
        ? '已开启启动后隐藏窗口，下次启动生效。'
        : '已关闭启动后隐藏窗口，下次启动显示主窗口。',
      'success',
    )
  } catch {
    notify('更改启动后隐藏窗口未成功确认，请重新读取当前设置。', 'error')
  }
}

function refreshStatus() {
  if (busy.value) return
  void status.refetch()
  void visibility.refetch()
}
</script>

<template>
  <UiPanel aria-labelledby="startup-heading">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 id="startup-heading" class="text-base font-semibold">启动设置</h2>
      <UiButton
        variant="secondary"
        size="small"
        :disabled="busy"
        :loading="busy"
        @click="refreshStatus"
      >
        刷新状态
      </UiButton>
    </div>
    <div class="divide-y divide-border-hairline">
      <UiAsyncSection
        :pending="status.isPending.value || (status.isError.value && status.isFetching.value)"
        :error="status.error.value"
        loading-text="正在读取开机自启动状态…"
        error-text="无法读取开机自启动状态，请重新读取后再试。"
        retry-label="重新读取"
        dense
        @retry="refreshStatus"
      >
        <div>
          <SwitchRoot
            id="autostart-switch"
            class="group flex min-h-20 w-full items-center justify-between gap-5 py-3 text-left focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-60"
            aria-label="启用开机自启动"
            aria-describedby="autostart-description"
            :model-value="(status.data.value?.enabled && !status.data.value.needsRepair) ?? false"
            :disabled="autostartDisabled"
            :aria-busy="setAutostart.isPending.value"
            @update:model-value="changeAutostart"
          >
            <span class="flex min-w-0 flex-col gap-1">
              <strong class="text-sm">启用开机自启动</strong>
              <span id="autostart-description" class="text-xs text-text-tertiary">
                登录桌面会话后自动启动 dan-tick
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
          <p
            v-if="setAutostart.isPending.value"
            role="status"
            class="pb-3 text-xs text-text-secondary"
          >
            正在更新开机自启动状态…
          </p>
          <UiAlert v-if="!status.data.value?.supported" tone="warning" role="note">
            当前系统不支持开机自启动。
          </UiAlert>
          <UiAlert v-else-if="status.data.value?.needsRepair" tone="warning" role="note">
            <p>已有启动项需要修复，重新开启即可更新；也可以清理启动项。</p>
            <UiButton
              variant="secondary"
              size="small"
              class="mt-2"
              :disabled="autostartDisabled"
              @click="changeAutostart(false)"
            >
              清理启动项
            </UiButton>
          </UiAlert>
        </div>
      </UiAsyncSection>
      <UiAsyncSection
        :pending="
          visibility.isPending.value || (visibility.isError.value && visibility.isFetching.value)
        "
        :error="visibility.error.value"
        loading-text="正在读取启动后隐藏窗口设置…"
        error-text="无法读取启动后隐藏窗口设置，请重新读取后再试。"
        retry-label="重新读取"
        dense
        @retry="refreshStatus"
      >
        <div>
          <SwitchRoot
            id="startup-visibility-switch"
            class="group flex min-h-20 w-full items-center justify-between gap-5 py-3 text-left focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-60"
            aria-label="启动后隐藏窗口"
            aria-describedby="startup-visibility-description"
            :model-value="hideOnStartup"
            :disabled="visibilityDisabled"
            :aria-busy="setVisibility.isPending.value"
            @update:model-value="changeStartupVisibility"
          >
            <span class="flex min-w-0 flex-col gap-1">
              <strong class="text-sm">启动后隐藏窗口</strong>
              <span id="startup-visibility-description" class="text-xs text-text-tertiary">
                启动后不显示主窗口，仅保留系统托盘入口
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
          <p
            v-if="setVisibility.isPending.value"
            role="status"
            class="pb-3 text-xs text-text-secondary"
          >
            正在更新启动后隐藏窗口设置…
          </p>
          <p class="pb-3 text-xs text-text-tertiary">保存后在下一次启动 dan-tick 时生效。</p>
        </div>
      </UiAsyncSection>
    </div>
  </UiPanel>
</template>
