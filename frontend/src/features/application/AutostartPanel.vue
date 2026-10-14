<script setup lang="ts">
import { computed } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiAsyncSection from '@/components/ui/UiAsyncSection.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiPanel from '@/components/ui/UiPanel.vue'
import { useToast } from '@/composables/useToast'
import { useAutostartStatus, useSetAutostartEnabled } from './queries'

const status = useAutostartStatus()
const setAutostart = useSetAutostartEnabled()
const busy = computed(() => status.isFetching.value || setAutostart.isPending.value)
const disabled = computed(() => busy.value || status.isError.value || !status.data.value?.supported)
const { notify, dismiss } = useToast()

async function changeAutostart(enabled: boolean) {
  if (disabled.value) return
  dismiss()
  try {
    await setAutostart.mutateAsync(enabled)
    notify(enabled ? '已开启开机自启动。' : '已关闭开机自启动。', 'success')
  } catch {
    notify('更改开机自启动未成功确认，请查看当前状态或重新读取。', 'error')
  }
}

function refreshStatus() {
  if (!busy.value) void status.refetch()
}
</script>

<template>
  <UiPanel aria-labelledby="autostart-heading">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 id="autostart-heading" class="text-base font-semibold">开机自启动</h2>
      <UiButton
        v-if="!status.isPending.value && !status.isError.value"
        variant="secondary"
        size="small"
        :disabled="busy"
        :loading="status.isFetching.value"
        @click="refreshStatus"
      >
        {{ status.isFetching.value ? '正在读取…' : '刷新状态' }}
      </UiButton>
    </div>
    <UiAsyncSection
      :pending="status.isPending.value || (status.isError.value && status.isFetching.value)"
      :error="status.error.value"
      loading-text="正在读取开机自启动状态…"
      error-text="无法读取开机自启动状态，请重新读取后再试。"
      retry-label="重新读取"
      dense
      @retry="refreshStatus"
    >
      <SwitchRoot
        id="autostart-switch"
        class="group flex min-h-20 w-full items-center justify-between gap-5 py-3 text-left focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-60"
        aria-label="启用开机自启动"
        aria-describedby="autostart-description"
        :model-value="(status.data.value?.enabled && !status.data.value.needsRepair) ?? false"
        :disabled="disabled"
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
      <p v-if="setAutostart.isPending.value" role="status" class="text-xs text-text-secondary">
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
          :disabled="disabled"
          @click="changeAutostart(false)"
        >
          清理启动项
        </UiButton>
      </UiAlert>
    </UiAsyncSection>
  </UiPanel>
</template>
