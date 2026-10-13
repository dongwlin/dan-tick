<script setup lang="ts">
import { nextTick, useTemplateRef, watch } from 'vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import UiField from '@/components/ui/UiField.vue'
import { fieldClass } from '@/components/ui/uiStyles'

const props = defineProps<{
  open: boolean
  /** Identifies the room under edit; only read while the Dialog is open. */
  roomId: number | null
  /** Supporting line naming the room, so the Dialog is unambiguous on a long page. */
  roomLabel: string
  pending: boolean
  /** Safe binding error rendered inside the Dialog; the page keeps other messages. */
  error?: string
}>()

const emit = defineEmits<{
  submit: [remark: string]
  'update:open': [open: boolean]
  /** Empty message clears the binding error once the user edits the field again. */
  'update:error': [message: string]
}>()

const remark = defineModel<string>('remark', { required: true })
const remarkInput = useTemplateRef<HTMLInputElement>('remarkInput')
const errorBox = useTemplateRef<HTMLElement>('errorBox')

// A rejection must be seen: move focus to the message, then drop it as soon as
// the user edits the field the attempt depends on (规范 5.3 / 8.3).
watch(
  () => props.error,
  async (message) => {
    if (!message) return
    await nextTick()
    errorBox.value?.focus()
  },
)
watch(remark, () => {
  if (props.error) emit('update:error', '')
})

function submit() {
  if (props.roomId === null || props.pending) return
  emit('submit', remark.value.trim())
}

function focusRemark(event: Event) {
  event.preventDefault()
  remarkInput.value?.focus()
}
</script>

<template>
  <UiDialog
    :open="open"
    title="修改备注"
    :description="roomLabel"
    :busy="pending"
    @open-auto-focus="focusRemark"
    @update:open="emit('update:open', $event)"
  >
    <form class="space-y-4" @submit.prevent="submit">
      <UiField id="remark-input" v-slot="{ id, ariaInvalid, describedBy }" label="备注">
        <input
          :id="id"
          ref="remarkInput"
          v-model="remark"
          :class="fieldClass"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedBy"
          maxlength="100"
          autocomplete="off"
          placeholder="可选"
        />
      </UiField>
      <div v-if="error" ref="errorBox" tabindex="-1" class="rounded-[10px]">
        <UiAlert tone="danger" show-icon>保存失败：{{ error }}</UiAlert>
      </div>
      <div class="flex items-center justify-end gap-2">
        <p v-if="pending" role="status" class="mr-auto text-xs text-text-tertiary">
          正在保存，请稍候…
        </p>
        <UiButton type="button" :disabled="pending" @click="emit('update:open', false)"
          >取消</UiButton
        >
        <UiButton type="submit" variant="primary" :loading="pending">保存备注</UiButton>
      </div>
    </form>
  </UiDialog>
</template>
