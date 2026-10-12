<script setup lang="ts">
import { computed } from 'vue'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import UiAvatar from '@/components/ui/UiAvatar.vue'
import { flatPanel } from '@/components/ui/uiStyles'
import { useAccountStatus } from './queries'

const account = useAccountStatus()
const loggedIn = computed(() => account.data.value?.state === State.LoggedIn)
const name = computed(() => {
  if (account.isPending.value) return '正在读取账号…'
  if (loggedIn.value) return account.data.value?.name || 'B 站用户'
  if (account.data.value?.state === State.Expired) return '登录已过期'
  if (account.data.value?.state === State.ReauthenticationRequired) return '需要重新扫码登录'
  if (account.data.value?.state === State.VerificationFailed || account.isError.value)
    return '账号信息暂不可用'
  return '未登录 B 站'
})
const uid = computed(() => (loggedIn.value ? account.data.value?.uid || '暂不可用' : '—'))
</script>

<template>
  <section :class="[flatPanel, 'flex min-w-0 items-center gap-4 px-5 py-5']" aria-label="账号资料">
    <UiAvatar
      :url="loggedIn ? account.data.value?.avatarUrl : undefined"
      class="size-15 border border-border-hairline"
    />
    <div class="min-w-0 flex-1">
      <p class="truncate text-lg font-semibold text-text-primary" :title="name">{{ name }}</p>
      <p class="mt-1 text-sm text-text-secondary">UID：{{ uid }}</p>
    </div>
  </section>
</template>
