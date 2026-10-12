<script setup lang="ts">
import { shallowRef } from 'vue'
import { Plus, X } from 'lucide-vue-next'
import { fieldClass } from '@/components/uiStyles'
import { primaryButton, secondaryButton } from '@/components/buttonStyles'

defineProps<{ pending: boolean }>()
const emit = defineEmits<{ submit: [value: string]; cancel: [] }>()
const value = shallowRef('')
function submit() {
  if (value.value.trim()) emit('submit', value.value.trim())
}
</script>

<template>
  <form class="acrylic rounded-[14px] p-5" @submit.prevent="submit">
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-base font-semibold">添加直播间</h2>
      <button
        type="button"
        class="flex size-11 items-center justify-center rounded-[10px] text-text-secondary hover:bg-surface-muted active:bg-surface-solid-hover"
        aria-label="关闭添加表单"
        @click="emit('cancel')"
      >
        <X class="size-5" aria-hidden="true" />
      </button>
    </div>
    <span class="block text-sm font-medium text-text-secondary">直播间 ID 或链接</span>
    <input
      id="room-input"
      v-model="value"
      aria-label="直播间 ID 或链接"
      :class="fieldClass"
      class="mt-2"
      required
      autocomplete="off"
      placeholder="例如 22625025 或 https://live.bilibili.com/22625025"
    />
    <p class="mt-2 text-xs text-text-tertiary">主播与标题由 B 站查询，备注可添加后修改。</p>
    <div class="mt-4 flex justify-end gap-2">
      <button type="button" :class="secondaryButton" @click="emit('cancel')">取消</button>
      <button type="submit" :disabled="pending" :aria-busy="pending" :class="primaryButton">
        <Plus class="size-4" aria-hidden="true" />
        {{ pending ? '正在验证…' : '添加到列表' }}
      </button>
    </div>
  </form>
</template>
