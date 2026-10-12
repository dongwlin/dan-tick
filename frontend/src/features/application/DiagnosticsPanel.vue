<script setup lang="ts">
import { shallowRef } from 'vue'
import { FolderOpen, FileArchive } from 'lucide-vue-next'
import UiToast from '@/components/UiToast.vue'
import { secondaryButton } from '@/components/buttonStyles'
import {
  useDiagnosticsStatus,
  useExportDiagnostics,
  useOpenDataDirectory,
  useOpenLogDirectory,
} from './queries'

const status = useDiagnosticsStatus()
const logs = useOpenLogDirectory()
const data = useOpenDataDirectory()
const exportAction = useExportDiagnostics()
const toast = shallowRef<{
  id: number
  message: string
  tone: 'success' | 'error' | 'info'
} | null>(null)
let nextToastId = 0

function notify(message: string, tone: 'success' | 'error' | 'info') {
  toast.value = { id: ++nextToastId, message, tone }
}

async function run(action: 'logs' | 'data' | 'export') {
  toast.value = null
  try {
    if (action === 'export') {
      const result = await exportAction.mutateAsync()
      if (result.cancelled) {
        notify('已取消导出。', 'info')
      } else {
        notify(`诊断包已保存为 ${result.filename}。分享前请检查包内内容。`, 'success')
      }
    } else {
      await (action === 'logs' ? logs.mutateAsync() : data.mutateAsync())
      notify(action === 'logs' ? '已请求打开日志目录。' : '已请求打开数据目录。', 'success')
    }
  } catch {
    notify(
      action === 'export'
        ? '诊断包导出失败或未通过安全检查，请查看本地日志后重试。'
        : '无法打开目录，请检查系统文件管理器和目录权限。',
      'error',
    )
  }
  if (action === 'export') void status.refetch()
}
</script>

<template>
  <section class="acrylic rounded-[14px] p-5" aria-label="数据与日志">
    <h2 class="mb-2 text-base font-semibold">数据与日志</h2>
    <div
      class="flex flex-wrap items-center justify-between gap-4 border-b border-border-hairline py-4"
    >
      <div class="min-w-0 flex-1">
        <strong class="text-sm">本地数据目录</strong>
        <p v-if="status.data.value" class="mt-1 break-all text-xs text-text-tertiary">
          {{ status.data.value.dataDirectory }}
        </p>
      </div>
      <button
        type="button"
        :class="secondaryButton"
        :disabled="data.isPending.value"
        :aria-busy="data.isPending.value"
        @click="run('data')"
      >
        <FolderOpen class="size-4" aria-hidden="true" />打开数据目录
      </button>
    </div>
    <div class="flex flex-wrap items-center justify-between gap-4 py-4">
      <div class="min-w-0 flex-1">
        <strong class="text-sm">诊断日志目录</strong>
        <p v-if="status.data.value" class="mt-1 break-all text-xs text-text-tertiary">
          {{ status.data.value.directory }}
        </p>
      </div>
      <button
        type="button"
        :class="secondaryButton"
        :disabled="logs.isPending.value"
        :aria-busy="logs.isPending.value"
        @click="run('logs')"
      >
        <FolderOpen class="size-4" aria-hidden="true" />打开日志目录
      </button>
    </div>
    <p v-if="status.data.value?.degraded" role="alert" class="mb-3 text-sm text-soft-warning-fg">
      日志写入已降级，请检查日志目录权限或剩余空间；后台任务仍可运行。
    </p>
    <p v-if="status.isError.value" role="alert" class="mb-3 text-sm text-soft-warning-fg">
      无法读取日志状态。
    </p>
    <button
      type="button"
      :class="secondaryButton"
      :disabled="exportAction.isPending.value"
      :aria-busy="exportAction.isPending.value"
      @click="run('export')"
    >
      <FileArchive class="size-4" aria-hidden="true" />
      {{ exportAction.isPending.value ? '正在检查并导出…' : '导出诊断包…' }}
    </button>
    <UiToast
      v-if="toast"
      :key="toast.id"
      :message="toast.message"
      :tone="toast.tone"
      @dismiss="toast = null"
    />
  </section>
</template>
