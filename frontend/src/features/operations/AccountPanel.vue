<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import { ShieldCheck, RefreshCw } from 'lucide-vue-next'
import { bindingError } from '@/features/configuration/queries'
import { flatPanel, secondaryButton } from '@/features/configuration/style'
import { accountStatusLabels, operationKeys, useAccountStatus, useClearAccount } from './queries'
import QRLoginCard from './QRLoginCard.vue'
import { useQRLogin } from './useQRLogin'

const status = useAccountStatus()
const client = useQueryClient()
const clear = useClearAccount()
const feedback = shallowRef('')
const errorMessage = shallowRef('')
const qr = useQRLogin({
  onConfirmed: () => client.invalidateQueries({ queryKey: operationKeys.account }),
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
  errorMessage.value = ''
  feedback.value = ''
  await qr.begin()
}

async function clearCredentials() {
  if (!window.confirm('清除已保存的 B 站登录凭证？已创建的任务不会删除。')) return
  errorMessage.value = ''
  feedback.value = ''
  try {
    await clear.mutateAsync()
    feedback.value = '账号凭证已清除。'
  } catch (error) {
    errorMessage.value = bindingError(error)
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
    <QRLoginCard
      :state="qr.state.value"
      :qr-image="qr.qrImage.value"
      :candidate-u-i-d="qr.candidateUID.value"
      :busy="qr.busy.value"
      :cancel-pending="qr.cancelPending.value"
      :disabled="clear.isPending.value"
      :error-message="qr.errorMessage.value"
      @begin="beginLogin"
      @cancel="qr.cancel"
      @confirm="qr.confirmAccount"
    />
    <div class="flex flex-wrap gap-2 border-t border-border-hairline pt-4">
      <button
        type="button"
        :class="[secondaryButton, 'min-h-11']"
        :disabled="status.isFetching.value || qr.active.value || qr.busy.value"
        @click="status.refetch()"
      >
        <RefreshCw class="size-4" aria-hidden="true" />检查登录状态
      </button>
      <button
        type="button"
        :class="[secondaryButton, 'min-h-11']"
        :disabled="clear.isPending.value || qr.active.value || qr.busy.value"
        @click="clearCredentials"
      >
        清除凭证
      </button>
    </div>
    <p v-if="feedback" aria-live="polite" class="text-sm text-text-secondary">{{ feedback }}</p>
    <p v-if="errorMessage" role="alert" class="text-sm text-soft-danger-fg">{{ errorMessage }}</p>
  </section>
</template>
