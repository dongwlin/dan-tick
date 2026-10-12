<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import { ShieldCheck, RefreshCw } from 'lucide-vue-next'
import { flatPanel } from '@/components/ui/uiStyles'
import { secondaryButton } from '@/components/ui/buttonStyles'
import UiConfirmDialog from '@/components/ui/UiConfirmDialog.vue'
import { useToast } from '@/composables/useToast'
import { bindingError } from '@/lib/bindingError'
import { accountStatusLabels, operationKeys, useAccountStatus, useClearAccount } from './queries'
import QRLoginCard from './QRLoginCard.vue'
import { useQRLogin } from './useQRLogin'

const status = useAccountStatus()
const client = useQueryClient()
const clear = useClearAccount()
const dialogOpen = shallowRef(false)
const clearDialogOpen = shallowRef(false)
const { notify, dismiss } = useToast()

const qr = useQRLogin({
  onConfirmed: async () => {
    dialogOpen.value = false
    notify('扫码登录成功', 'success')
    await client.invalidateQueries({ queryKey: operationKeys.account })
  },
})
const statusLabel = computed(() => {
  if (status.isFetching.value) return '验证中…'
  if (status.isError.value) return '状态暂不可用'
  return accountStatusLabels[status.data.value?.state ?? ''] ?? '暂不可用'
})
const needsReauthentication = computed(
  () => status.data.value?.state === State.ReauthenticationRequired,
)

async function beginLogin() {
  if (clear.isPending.value || qr.busy.value || qr.active.value) return
  dialogOpen.value = true
  dismiss()
  await qr.begin()
}

async function setDialogOpen(open: boolean) {
  if (open) {
    await beginLogin()
    return
  }
  if (qr.cancelPending.value || (qr.state.value === 'account_mismatch' && qr.busy.value)) return
  await qr.cancel()
  dialogOpen.value = false
  if (qr.errorMessage.value) notify(qr.errorMessage.value, 'error')
}

function requestClearCredentials() {
  clearDialogOpen.value = true
}

async function clearCredentials() {
  clearDialogOpen.value = false
  dismiss()
  try {
    await clear.mutateAsync()
    notify('账号凭证已清除。', 'success')
  } catch (error) {
    notify(bindingError(error), 'error')
  }
}
</script>

<template>
  <section :class="[flatPanel, 'space-y-4 p-5']" aria-labelledby="account-heading">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 id="account-heading" class="text-base font-semibold">B站账号凭证</h2>
      <span class="flex items-center gap-2 text-sm text-text-secondary" aria-live="polite">
        <ShieldCheck class="size-4" aria-hidden="true" />
        {{ statusLabel }}
      </span>
    </div>
    <p v-if="needsReauthentication" class="text-sm text-soft-warning-fg">
      现有凭证需要重新扫码绑定。原凭证和任务会保留，扫码成功后更新账号。
    </p>
    <div class="flex flex-wrap gap-2">
      <QRLoginCard
        :open="dialogOpen"
        :state="qr.state.value"
        :qr-image="qr.qrImage.value"
        :candidate-u-i-d="qr.candidateUID.value"
        :busy="qr.busy.value"
        :cancel-pending="qr.cancelPending.value"
        :disabled="clear.isPending.value"
        :error-message="qr.errorMessage.value"
        @begin="beginLogin"
        @update:open="setDialogOpen"
        @confirm="qr.confirmAccount"
      />
      <button
        type="button"
        :class="[secondaryButton, 'min-h-11']"
        :disabled="status.isFetching.value || qr.active.value || qr.busy.value"
        :aria-busy="status.isFetching.value"
        @click="status.refetch()"
      >
        <RefreshCw class="size-4" aria-hidden="true" />检查登录状态
      </button>
      <button
        type="button"
        :class="[secondaryButton, 'min-h-11']"
        :disabled="clear.isPending.value || qr.active.value || qr.busy.value"
        :aria-busy="clear.isPending.value"
        @click="requestClearCredentials"
      >
        清除凭证
      </button>
    </div>
    <UiConfirmDialog
      :open="clearDialogOpen"
      title="清除 B 站登录凭证"
      description="清除已保存的 B 站登录凭证？已创建的任务不会删除。"
      confirm-label="清除凭证"
      variant="danger"
      @update:open="clearDialogOpen = $event"
      @confirm="clearCredentials"
    />
  </section>
</template>
