<script setup lang="ts">
import { computed, nextTick, shallowRef, useTemplateRef } from 'vue'
import { Send } from 'lucide-vue-next'
import { Outcome } from '@bindings/github.com/dongwlin/dan-tick/internal/danmaku/models'
import UiButton from '@/components/ui/UiButton.vue'
import UiField from '@/components/ui/UiField.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { useRoomOptions } from '@/features/configuration/queries'
import { fieldClass } from '@/components/ui/uiStyles'
import UiPanel from '@/components/ui/UiPanel.vue'
import { useToast } from '@/composables/useToast'
import { bindingError } from '@/lib/bindingError'
import { danmakuMessageLimitDescription, validateDanmakuMessage } from '@/lib/danmakuMessage'
import { sendFailureText, useTestSend } from './queries'

const rooms = useRoomOptions()
const send = useTestSend()
const roomID = shallowRef(0)
const roomOptions = computed(() =>
  (rooms.data.value ?? []).map((room) => ({
    value: room.id,
    label: room.remark || room.title || `房间 ${room.platformRoomId}`,
  })),
)
const message = shallowRef('')
const submitted = shallowRef(false)
const roomError = computed(() => (submitted.value && !roomID.value ? '请选择直播间' : ''))
const messageError = computed(() =>
  submitted.value ? validateDanmakuMessage(message.value) : undefined,
)
const messageInput = useTemplateRef<HTMLTextAreaElement>('message-input')
const { notify, dismiss } = useToast()

async function submit() {
  if (send.isPending.value) return
  dismiss()
  submitted.value = true
  if (roomError.value || messageError.value) {
    await nextTick()
    if (roomError.value) document.getElementById('test-room-id')?.focus()
    else messageInput.value?.focus()
    return
  }
  send.reset()
  try {
    const result = await send.mutateAsync({ roomId: roomID.value, message: message.value })
    if (result.outcome === Outcome.Accepted) {
      notify('测试弹幕已被 B 站接受，已记入执行历史。', 'success')
    } else if (result.outcome === Outcome.Unknown) {
      notify('发送结果无法确认，请勿立即重试；本次测试已记入执行历史。', 'warning')
    } else {
      notify(sendFailureText[result.errorCode] ?? '测试发送未成功，请检查账号及弹幕内容。', 'error')
    }
  } catch (error) {
    notify(bindingError(error), 'error')
  }
}
</script>

<template>
  <UiPanel class="space-y-4" aria-labelledby="send-heading">
    <div>
      <h2 id="send-heading" class="text-base font-semibold">测试发送</h2>
    </div>
    <form class="space-y-3" novalidate @submit.prevent="submit">
      <UiField
        id="test-room-id"
        v-slot="{ id, ariaInvalid, describedBy }"
        label="目标直播间"
        :error="roomError"
      >
        <UiSelect
          :id="id"
          v-model="roomID"
          :options="roomOptions"
          placeholder="请选择已保存的直播间"
          :class="fieldClass"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedBy"
          required
        />
      </UiField>
      <UiField
        id="test-message"
        v-slot="{ id, ariaInvalid, describedBy }"
        label="测试弹幕内容"
        :error="messageError"
      >
        <textarea
          :id="id"
          ref="message-input"
          v-model="message"
          :class="fieldClass"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedBy"
          rows="5"
          required
          :placeholder="`输入要实际发送到直播间的内容\n\n${danmakuMessageLimitDescription}实际发送仍受平台规则限制。`"
        />
      </UiField>
      <UiButton
        type="submit"
        variant="primary"
        class="min-w-36"
        :loading="send.isPending.value"
        :disabled="!roomID || !message.trim()"
      >
        <Send class="size-4" aria-hidden="true" />发送测试弹幕
      </UiButton>
    </form>
    <p v-if="rooms.isError.value" role="alert" class="text-sm text-soft-danger-fg">
      {{ bindingError(rooms.error.value) }}
    </p>
  </UiPanel>
</template>
