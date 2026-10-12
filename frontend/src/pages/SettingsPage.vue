<script setup lang="ts">
import { AlertTriangle, Check } from 'lucide-vue-next'
import { RadioGroupIndicator, RadioGroupItem, RadioGroupRoot } from 'reka-ui'
import { useTheme, type ThemePreference } from '@/app/theme'
import AccountPanel from '@/features/operations/AccountPanel.vue'
import TestSendPanel from '@/features/operations/TestSendPanel.vue'
import CloseBehaviorPanel from '@/features/operations/CloseBehaviorPanel.vue'
import SchedulingPanel from '@/features/application/SchedulingPanel.vue'
import DiagnosticsPanel from '@/features/application/DiagnosticsPanel.vue'

const { preference, storageAvailable, setPreference } = useTheme()
const choices: { value: ThemePreference; label: string }[] = [
  { value: 'dark', label: '深色' },
  { value: 'light', label: '浅色' },
  { value: 'system', label: '跟随系统' },
]

function updateTheme(value: unknown): void {
  if (value === 'dark' || value === 'light' || value === 'system') {
    setPreference(value)
  }
}
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col gap-4">
    <h1 class="mx-auto w-full max-w-[1520px] px-7 max-[1100px]:px-5">设置</h1>
    <div class="min-h-0 flex-1 overflow-y-auto" tabindex="0" aria-label="设置内容">
      <div class="mx-auto w-full max-w-[1520px] space-y-5 px-7 pb-4 max-[1100px]:px-5">
        <AccountPanel />
        <TestSendPanel />
        <CloseBehaviorPanel />
        <SchedulingPanel />
        <section
          class="rounded-2xl border border-border-hairline bg-surface-solid p-5"
          aria-labelledby="theme-heading"
        >
          <h2 id="theme-heading" class="mb-4 text-lg font-semibold">界面主题</h2>
          <RadioGroupRoot
            :model-value="preference"
            orientation="horizontal"
            aria-label="界面主题"
            class="flex flex-wrap gap-3"
            @update:model-value="updateTheme"
          >
            <RadioGroupItem
              v-for="choice in choices"
              :key="choice.value"
              :value="choice.value"
              class="group flex min-h-11 items-center gap-2 rounded-[10px] border border-border-strong bg-surface-solid px-4 py-2.5 text-text-primary hover:bg-surface-solid-hover data-[state=checked]:border-soft-accent-border data-[state=checked]:bg-soft-accent-bg"
            >
              <span
                class="flex size-4 items-center justify-center rounded-full border border-border-strong group-data-[state=checked]:border-accent-solid"
                aria-hidden="true"
              >
                <RadioGroupIndicator
                  class="flex size-3 items-center justify-center text-accent-text"
                >
                  <Check class="size-3" :stroke-width="2.5" aria-hidden="true" />
                </RadioGroupIndicator>
              </span>
              <span>{{ choice.label }}</span>
            </RadioGroupItem>
          </RadioGroupRoot>
          <p
            v-if="!storageAvailable"
            class="mt-3 flex items-center gap-2 rounded-[10px] border border-soft-warning-border bg-soft-warning-bg px-3 py-2 text-sm text-soft-warning-fg"
            role="alert"
          >
            <AlertTriangle class="size-4 shrink-0" aria-hidden="true" />
            当前环境无法保存主题偏好，本次打开期间仍可切换。
          </p>
        </section>
        <DiagnosticsPanel />
      </div>
    </div>
  </section>
</template>
