<script setup lang="ts">
import { shallowRef } from 'vue'
import { Send } from 'lucide-vue-next'
import { Outcome } from '@bindings/github.com/dongwlin/dan-tick/internal/danmaku/models'
import UiToast from '@/components/UiToast.vue'
import { useRoomOptions } from '@/features/configuration/queries'
import { fieldClass, flatPanel, labelClass } from '@/components/uiStyles'
import { primaryButton } from '@/components/buttonStyles'
import { bindingError } from '@/lib/bindingError'
import { sendFailureText, useTestSend } from './queries'

const rooms = useRoomOptions()
const send = useTestSend()
const roomID = shallowRef(0)
const message = shallowRef('')
const toast = shallowRef<{
  id: number
  message: string
  tone: 'success' | 'error' | 'warning'
} | null>(null)
let nextToastId = 0

function notify(message: string, tone: 'success' | 'error' | 'warning') {
  toast.value = { id: ++nextToastId, message, tone }
}

async function submit() {
  toast.value = null
  send.reset()
  try {
    const result = await send.mutateAsync({ roomId: roomID.value, message: message.value })
    if (result.outcome === Outcome.Accepted) {
      notify('测试弹幕已被 B 站接受（不计入任务执行记录）。', 'success')
    } else if (result.outcome === Outcome.Unknown) {
      notify('发送结果无法确认，请勿立即重试。', 'warning')
    } else {
      notify(sendFailureText[result.errorCode] ?? '测试发送未成功，请检查账号及弹幕内容。', 'error')
    }
  } catch (error) {
    notify(bindingError(error), 'error')
  }
}
</script>

<template>
  <section :class="[flatPanel, 'space-y-4 p-5']" aria-labelledby="send-heading">
    <div>
      <h2 id="send-heading" class="text-base font-semibold">测试发送</h2>
      <p class="mt-1 text-sm text-text-tertiary">
        真实发送一条弹幕，与任务共用账号限流；不写入任务执行历史
      </p>
    </div>
    <form class="space-y-3" @submit.prevent="submit">
      <label for="test-room-id" :class="labelClass">
        目标直播间
        <select id="test-room-id" v-model.number="roomID" :class="fieldClass" required>
          <option :value="0" disabled>请选择已保存的直播间</option>
          <option v-for="room in rooms.data.value ?? []" :key="room.id" :value="room.id">
            {{ room.remark || room.title || `房间 ${room.platformRoomId}` }}
          </option>
        </select>
      </label>
      <label for="test-message" :class="labelClass">
        测试弹幕内容
        <textarea
          id="test-message"
          v-model="message"
          :class="fieldClass"
          rows="2"
          required
          placeholder="输入要实际发送到直播间的内容"
        />
      </label>
      <button
        type="submit"
        :class="primaryButton"
        :disabled="send.isPending.value || !roomID || !message.trim()"
        :aria-busy="send.isPending.value"
      >
        <Send class="size-4" aria-hidden="true" />{{
          send.isPending.value ? '正在发送…' : '发送测试弹幕'
        }}
      </button>
    </form>
    <p v-if="rooms.isError.value" role="alert" class="text-sm text-soft-danger-fg">
      {{ bindingError(rooms.error.value) }}
    </p>
    <UiToast
      v-if="toast"
      :key="toast.id"
      :message="toast.message"
      :tone="toast.tone"
      @dismiss="toast = null"
    />
  </section>
</template>
