<script setup lang="ts">
import { shallowRef } from 'vue'
import { Plus, X } from 'lucide-vue-next'
import UiButton from '@/components/UiButton.vue'
import UiField from '@/components/UiField.vue'
import { fieldClass } from '@/components/uiStyles'

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
    <UiField
      id="room-input"
      v-slot="{ id, ariaInvalid, describedBy }"
      label="直播间 ID 或链接"
      description="主播与标题由 B 站查询，备注可添加后修改。"
    >
      <input
        :id="id"
        v-model="value"
        :class="fieldClass"
        :aria-invalid="ariaInvalid"
        :aria-describedby="describedBy"
        required
        autocomplete="off"
        placeholder="例如 22625025 或 https://live.bilibili.com/22625025"
      />
    </UiField>
    <div class="mt-4 flex justify-end gap-2">
      <UiButton @click="emit('cancel')">取消</UiButton>
      <UiButton type="submit" variant="primary" :loading="pending">
        <Plus class="size-4" aria-hidden="true" />添加到列表
      </UiButton>
    </div>
  </form>
</template>
