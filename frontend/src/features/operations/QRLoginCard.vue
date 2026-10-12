<script setup lang="ts">
import { computed } from 'vue'
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
  DialogTrigger,
} from 'reka-ui'
import { QrCode, Smartphone, X } from 'lucide-vue-next'
import { primaryButton } from '@/components/ui/buttonStyles'
import UiAlert from '@/components/ui/UiAlert.vue'
import type { QRLoginState } from './useQRLogin'

const props = defineProps<{
  open: boolean
  state: QRLoginState
  qrImage: string
  candidateUID: string
  busy: boolean
  cancelPending: boolean
  disabled: boolean
  errorMessage: string
}>()
const emit = defineEmits<{ 'update:open': [open: boolean]; begin: []; confirm: [] }>()
const closingLocked = computed(
  () => props.cancelPending || (props.state === 'account_mismatch' && props.busy),
)
const retryable = computed(() => ['expired', 'failed', 'cancelled'].includes(props.state))
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
    case 'account_mismatch':
      return '扫码账号与当前绑定账号不同'
    case 'cancelled':
      return '已取消扫码登录'
    case 'failed':
      return '扫码登录未完成，请重试'
    default:
      return '正在准备二维码…'
  }
})
const retryText = computed(() => (props.state === 'expired' ? '重新获取二维码' : '重试扫码登录'))

function updateOpen(open: boolean) {
  if (!open && closingLocked.value) return
  emit('update:open', open)
}

function preventClosingWhenLocked(event: Event) {
  if (closingLocked.value) event.preventDefault()
}
</script>

<template>
  <DialogRoot :open="open" @update:open="updateOpen">
    <DialogTrigger as-child>
      <button
        type="button"
        :class="[primaryButton, 'min-h-11']"
        :disabled="disabled"
        :aria-disabled="busy"
      >
        <QrCode class="size-4" aria-hidden="true" />扫码登录
      </button>
    </DialogTrigger>
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-50 bg-[rgb(8_14_20/42%)]" />
      <DialogContent
        aria-modal="true"
        class="fixed top-1/2 left-1/2 z-50 max-h-[calc(100dvh_-_3rem)] w-104 max-w-[calc(100vw_-_3rem)] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl bg-surface-solid p-6 shadow-(--shadow-overlay)"
        @escape-key-down="preventClosingWhenLocked"
        @interact-outside="preventClosingWhenLocked"
      >
        <div class="mb-5 flex items-center justify-between gap-4">
          <DialogTitle class="text-lg font-semibold">扫码登录</DialogTitle>
          <DialogClose as-child>
            <button
              type="button"
              aria-label="关闭扫码登录"
              class="flex size-11 shrink-0 items-center justify-center rounded-[10px] text-text-secondary hover:bg-surface-solid-hover active:bg-surface-muted disabled:opacity-50"
              :disabled="closingLocked"
            >
              <X class="size-5" aria-hidden="true" />
            </button>
          </DialogClose>
        </div>
        <div class="space-y-5">
          <img
            v-if="state === 'waiting' && qrImage"
            :src="qrImage"
            alt="B 站登录二维码，请使用 B 站手机 App 扫描"
            width="224"
            height="224"
            class="mx-auto size-56"
          />
          <DialogDescription as-child>
            <p
              class="flex items-center justify-center gap-2 text-sm text-text-secondary"
              role="status"
            >
              <Smartphone v-if="state === 'scanned'" class="size-5 shrink-0" aria-hidden="true" />
              {{ statusText }}
            </p>
          </DialogDescription>
          <p v-if="state === 'account_mismatch'" class="text-sm text-text-secondary wrap-anywhere">
            本次扫码账号 UID：{{ candidateUID }}。确认切换后，现有任务将使用此账号发送弹幕。
          </p>
          <UiAlert
            v-if="errorMessage"
            tone="danger"
            variant="plain"
            role="alert"
            :show-icon="false"
          >
            {{ errorMessage }}
          </UiAlert>
          <div
            v-if="state === 'account_mismatch' || retryable"
            class="flex flex-wrap justify-end gap-2"
          >
            <button
              v-if="state === 'account_mismatch'"
              type="button"
              :class="[primaryButton, 'min-h-11']"
              :disabled="busy || disabled"
              :aria-busy="busy"
              @click="emit('confirm')"
            >
              {{ busy ? '正在切换…' : '确认切换账号' }}
            </button>
            <button
              v-else-if="retryable"
              type="button"
              :class="[primaryButton, 'min-h-11']"
              :disabled="busy || disabled"
              :aria-busy="busy"
              @click="emit('begin')"
            >
              {{ retryText }}
            </button>
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
