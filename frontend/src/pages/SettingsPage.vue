<script setup lang="ts">
import { AlertTriangle, Check, RotateCcw } from 'lucide-vue-next'
import { RadioGroupIndicator, RadioGroupItem, RadioGroupRoot } from 'reka-ui'
import { useTheme, type ThemePreference } from '@/app/theme'
import UiButton from '@/components/UiButton.vue'

const { preference, activeTheme, storageAvailable, setPreference } = useTheme()
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
  <section class="space-y-6">
    <h1>设置</h1>
    <fieldset
      class="rounded-[10px] border border-border-hairline bg-surface-solid p-5"
      aria-describedby="theme-description"
    >
      <legend class="px-1 text-base font-semibold">界面主题</legend>
      <p id="theme-description" class="mb-4 text-sm text-text-tertiary">
        仅保存显示偏好，不保存业务数据
      </p>
      <RadioGroupRoot
        :model-value="preference"
        orientation="horizontal"
        aria-label="界面主题"
        aria-describedby="theme-description"
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
            <RadioGroupIndicator class="flex size-3 items-center justify-center text-accent-text">
              <Check class="size-3" :stroke-width="2.5" aria-hidden="true" />
            </RadioGroupIndicator>
          </span>
          <span>{{ choice.label }}</span>
        </RadioGroupItem>
      </RadioGroupRoot>
      <p class="mt-3 text-sm text-text-secondary" role="status">
        当前显示：{{ activeTheme === 'dark' ? '深色' : '浅色' }}
      </p>
      <p
        v-if="!storageAvailable"
        class="mt-3 flex items-center gap-2 rounded-[10px] border border-soft-warning-border bg-soft-warning-bg px-3 py-2 text-sm text-soft-warning-fg"
        role="alert"
      >
        <AlertTriangle class="size-4 shrink-0" aria-hidden="true" />
        当前环境无法保存主题偏好，本次打开期间仍可切换。
      </p>
      <UiButton class="mt-4" :disabled="preference === 'dark'" @click="setPreference('dark')">
        <RotateCcw class="size-4" aria-hidden="true" />
        恢复默认主题
      </UiButton>
    </fieldset>
    <p>其他正式设置尚未接入，将在 F7 阶段按定稿原型迁移。</p>
  </section>
</template>
