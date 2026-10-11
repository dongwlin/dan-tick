<script setup lang="ts">
import { shallowRef } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { SaveAccount } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { ShieldCheck, RefreshCw } from 'lucide-vue-next'
import { bindingError } from '@/features/configuration/queries'
import {
  fieldClass,
  labelClass,
  panel,
  primaryButton,
  secondaryButton,
} from '@/features/configuration/style'
import { operationKeys, useAccountStatus, useClearAccount } from './queries'

const status = useAccountStatus()
const client = useQueryClient()
const saving = shallowRef(false)
const clear = useClearAccount()
const sessdata = shallowRef('')
const biliJct = shallowRef('')
const feedback = shallowRef('')
const errorMessage = shallowRef('')

const statusLabels: Record<string, string> = {
  unconfigured: '未配置',
  logged_in: '已登录',
  expired: '已过期',
  verification_failed: '验证失败',
}

async function saveCredentials() {
  if (saving.value) return
  saving.value = true
  errorMessage.value = ''
  feedback.value = ''
  try {
    // Do NOT pass Cookie values to a Vue Query mutation: its mutation cache
    // retains variables after settle. Only the Wails call sees this input.
    await SaveAccount({ sessdata: sessdata.value, biliJct: biliJct.value })
    // Never leave submitted Cookie values in a mounted form longer than needed.
    sessdata.value = ''
    biliJct.value = ''
    await client.invalidateQueries({ queryKey: operationKeys.account })
    feedback.value = '凭证已保存至系统安全存储。登录状态已重新检查。'
  } catch (error) {
    // Preserve both fields for correction, not in a Store or localStorage.
    errorMessage.value = bindingError(error)
  } finally {
    saving.value = false
  }
}

async function clearCredentials() {
  if (!window.confirm('清除已保存的 B 站登录凭证？已创建的任务不会删除。')) return
  errorMessage.value = ''
  feedback.value = ''
  try {
    await clear.mutateAsync()
    sessdata.value = ''
    biliJct.value = ''
    feedback.value = '账号凭证已清除。'
  } catch (error) {
    errorMessage.value = bindingError(error)
  }
}
</script>

<template>
  <section :class="[panel, 'space-y-4 p-5']" aria-labelledby="account-heading">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 id="account-heading" class="text-base font-semibold">B 站账号</h2>
        <p class="mt-1 text-sm text-text-tertiary">仅支持一个账号，凭证保存到系统安全存储</p>
      </div>
      <span class="flex items-center gap-2 text-sm text-text-secondary" aria-live="polite">
        <ShieldCheck class="size-4" aria-hidden="true" />
        {{
          status.isFetching.value
            ? '验证中…'
            : (statusLabels[status.data.value?.state ?? ''] ?? '暂不可用')
        }}
      </span>
    </div>
    <div class="grid gap-3 md:grid-cols-2">
      <label for="account-sessdata" :class="labelClass">
        SESSDATA
        <input
          id="account-sessdata"
          v-model="sessdata"
          type="password"
          autocomplete="off"
          :class="fieldClass"
          placeholder="输入 SESSDATA"
        />
      </label>
      <label for="account-bili-jct" :class="labelClass">
        bili_jct
        <input
          id="account-bili-jct"
          v-model="biliJct"
          type="password"
          autocomplete="off"
          :class="fieldClass"
          placeholder="输入 bili_jct"
        />
      </label>
    </div>
    <p class="text-xs text-text-tertiary">
      输入仅在本页临时保留；保存后清空。应用不会显示或读取回已存凭证。
    </p>
    <div class="flex flex-wrap gap-2">
      <button
        type="button"
        :class="primaryButton"
        :disabled="saving || clear.isPending.value"
        @click="saveCredentials"
      >
        {{ saving ? '保存中…' : '保存凭证' }}
      </button>
      <button
        type="button"
        :class="secondaryButton"
        :disabled="status.isFetching.value"
        @click="status.refetch()"
      >
        <RefreshCw class="size-4" aria-hidden="true" />检查登录状态
      </button>
      <button
        type="button"
        :class="secondaryButton"
        :disabled="clear.isPending.value || saving"
        @click="clearCredentials"
      >
        清除凭证
      </button>
    </div>
    <p v-if="feedback" aria-live="polite" class="text-sm text-text-secondary">{{ feedback }}</p>
    <p v-if="errorMessage" role="alert" class="text-sm text-soft-danger-fg">{{ errorMessage }}</p>
  </section>
</template>
