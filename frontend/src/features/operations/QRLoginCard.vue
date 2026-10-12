<script setup lang="ts">
import { computed, nextTick, useTemplateRef, watch } from 'vue'
import { CircleCheck, QrCode, Smartphone } from 'lucide-vue-next'
import { primaryButton, secondaryButton } from '@/features/configuration/style'
import type { QRLoginState } from './useQRLogin'

const props = defineProps<{
  state: QRLoginState
  qrImage: string
  candidateUID: string
  busy: boolean
  cancelPending: boolean
  disabled: boolean
  errorMessage: string
}>()
const emit = defineEmits<{ begin: []; cancel: []; confirm: [] }>()
const root = useTemplateRef('login-root')
const beginButton = useTemplateRef('begin-button')
const cancelButton = useTemplateRef('cancel-button')
const confirmButton = useTemplateRef('confirm-button')
const cancellable = computed(() =>
  ['preparing', 'waiting', 'scanned', 'account_mismatch'].includes(props.state),
)
const statusText = computed(() => {
  switch (props.state) {
    case 'preparing':
      return '正在准备二维码…'
    case 'waiting':
      return '等待扫码'
    case 'scanned':
      return '已扫码，请在手机上确认登录'
    case 'expired':
      return '二维码已过期，请重新获取'
    case 'confirmed':
      return '扫码登录成功'
    case 'account_mismatch':
      return '扫码账号与当前绑定账号不同'
    case 'cancelled':
      return '已取消扫码登录'
    case 'failed':
      return '扫码登录未完成，请重试'
    default:
      return '使用 B 站手机 App 扫码登录'
  }
})
const beginText = computed(() => {
  if (props.state === 'expired') return '重新获取二维码'
  if (props.state === 'failed') return '重试扫码登录'
  return '扫码登录'
})

watch(
  () => props.state,
  async () => {
    const focused = document.activeElement
    const ownedFocus = focused instanceof HTMLElement && root.value?.contains(focused)
    await nextTick()
    if (ownedFocus && !focused.isConnected) {
      const nextButton = confirmButton.value ?? beginButton.value ?? cancelButton.value
      nextButton?.focus()
    }
  },
)
</script>

<template>
  <div ref="login-root" class="flex flex-wrap items-center gap-x-6 gap-y-4" aria-label="扫码登录">
    <img
      v-if="state === 'waiting' && qrImage"
      :src="qrImage"
      alt="B 站登录二维码，请使用 B 站手机 App 扫描"
      width="176"
      height="176"
      class="size-44 shrink-0"
    />
    <div class="min-w-0 flex-1 basis-60 space-y-3">
      <p class="flex items-center gap-2 font-medium" role="status">
        <CircleCheck v-if="state === 'confirmed'" class="size-5 shrink-0" aria-hidden="true" />
        <Smartphone v-else-if="state === 'scanned'" class="size-5 shrink-0" aria-hidden="true" />
        <QrCode v-else class="size-5 shrink-0" aria-hidden="true" />
        {{ statusText }}
      </p>
      <p v-if="state === 'account_mismatch'" class="text-sm text-text-secondary wrap-anywhere">
        本次扫码账号 UID：{{ candidateUID }}。确认切换后，现有任务将使用此账号发送弹幕。
      </p>
      <p v-else-if="state === 'confirmed'" class="text-sm text-text-secondary">
        登录凭证已保存，可以检查账号状态或测试发送。
      </p>
      <p v-else class="max-w-prose text-sm text-text-secondary">
        在手机上确认登录。二维码仅用于本次登录，离开设置页会取消。
      </p>
      <div class="flex flex-wrap gap-2">
        <button
          v-if="state === 'account_mismatch'"
          ref="confirm-button"
          type="button"
          :class="[primaryButton, 'min-h-11']"
          :disabled="busy || disabled"
          @click="emit('confirm')"
        >
          {{ busy ? '正在切换…' : '确认切换账号' }}
        </button>
        <button
          v-else-if="!cancellable"
          ref="begin-button"
          type="button"
          :class="[primaryButton, 'min-h-11']"
          :disabled="busy || disabled"
          @click="emit('begin')"
        >
          {{ beginText }}
        </button>
        <button
          v-if="cancellable"
          ref="cancel-button"
          type="button"
          :class="[secondaryButton, 'min-h-11']"
          :disabled="cancelPending || (busy && state === 'account_mismatch')"
          @click="emit('cancel')"
        >
          {{ cancelPending ? '正在取消…' : '取消扫码' }}
        </button>
      </div>
      <p v-if="errorMessage" role="alert" class="text-sm text-soft-danger-fg">{{ errorMessage }}</p>
    </div>
  </div>
</template>
