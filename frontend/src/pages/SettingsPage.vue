<script setup lang="ts">
import { useTheme, type ThemePreference } from '@/app/theme'

const { preference, activeTheme, setPreference } = useTheme()
const choices: { value: ThemePreference; label: string }[] = [
  { value: 'dark', label: '深色' },
  { value: 'light', label: '浅色' },
  { value: 'system', label: '跟随系统' },
]
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
      <div class="flex flex-wrap gap-3">
        <label
          v-for="choice in choices"
          :key="choice.value"
          :for="`theme-${choice.value}`"
          class="flex min-h-11 cursor-pointer items-center gap-2 rounded-[10px] border border-border-strong bg-surface-solid px-4 py-2.5 hover:bg-surface-solid-hover has-checked:border-soft-accent-border has-checked:bg-soft-accent-bg"
        >
          <input
            :id="`theme-${choice.value}`"
            type="radio"
            name="theme"
            class="size-4 accent-accent-solid"
            :value="choice.value"
            :checked="preference === choice.value"
            @change="setPreference(choice.value)"
          />
          <span>{{ choice.label }}</span>
        </label>
      </div>
      <p class="mt-3 text-sm text-text-secondary" role="status">
        当前显示：{{ activeTheme === 'dark' ? '深色' : '浅色' }}
      </p>
    </fieldset>
    <p>其他正式设置尚未接入，将在 F7 阶段按定稿原型迁移。</p>
  </section>
</template>
