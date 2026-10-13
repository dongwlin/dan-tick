<script setup lang="ts">
import { nextTick, shallowRef, useTemplateRef, watch } from 'vue'
import { Plus } from 'lucide-vue-next'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import UiField from '@/components/ui/UiField.vue'
import { fieldClass } from '@/components/ui/uiStyles'

const props = defineProps<{
  open: boolean
  pending: boolean
  /** Safe binding error rendered inside the Dialog; the page keeps other messages. */
  error?: string
}>()

const emit = defineEmits<{
  submit: [input: { value: string; remark: string }]
  'update:open': [open: boolean]
  /** Empty message clears the binding error once the user edits a field again. */
  'update:error': [message: string]
}>()

const value = shallowRef('')
const remark = shallowRef('')
const valueInput = useTemplateRef<HTMLInputElement>('valueInput')
const errorBox = useTemplateRef<HTMLElement>('errorBox')

// Closing discards the draft: a reopened Dialog starts from empty fields, and a
// failed submission keeps them because the Dialog stays open.
watch(
  () => props.open,
  (open) => {
    if (open) return
    value.value = ''
    remark.value = ''
  },
)

// A rejection must be seen: move focus to the message, then drop it as soon as
// the user edits the fields the attempt depends on (规范 5.3 / 8.3).
watch(
  () => props.error,
  async (message) => {
    if (!message) return
    await nextTick()
    errorBox.value?.focus()
  },
)
watch([value, remark], () => {
  if (props.error) emit('update:error', '')
})

function submit() {
  const room = value.value.trim()
  if (!room || props.pending) return
  emit('submit', { value: room, remark: remark.value.trim() })
}

function focusFirstField(event: Event) {
  event.preventDefault()
  valueInput.value?.focus()
}
</script>

<template>
  <UiDialog
    :open="open"
    title="添加直播间"
    :busy="pending"
    @open-auto-focus="focusFirstField"
    @update:open="emit('update:open', $event)"
  >
    <form class="space-y-4" @submit.prevent="submit">
      <UiField id="room-input" v-slot="{ id, ariaInvalid, describedBy }" label="直播间 ID 或链接">
        <input
          :id="id"
          ref="valueInput"
          v-model="value"
          :class="fieldClass"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedBy"
          required
          autocomplete="off"
          placeholder="例如 22625025 或 https://live.bilibili.com/22625025"
        />
      </UiField>
      <UiField id="room-remark" v-slot="{ id, ariaInvalid, describedBy }" label="备注">
        <input
          :id="id"
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
        <UiAlert tone="danger" show-icon>{{ error }}</UiAlert>
      </div>
      <div class="flex items-center justify-end gap-2">
        <p v-if="pending" role="status" class="mr-auto text-xs text-text-tertiary">
          正在添加，请稍候…
        </p>
        <UiButton :disabled="pending" @click="emit('update:open', false)">取消</UiButton>
        <UiButton type="submit" variant="primary" :loading="pending">
          <Plus class="size-4" aria-hidden="true" />添加直播间
        </UiButton>
      </div>
    </form>
  </UiDialog>
</template>
