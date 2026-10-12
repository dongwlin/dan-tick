<script setup lang="ts">
import { shallowRef } from 'vue'
import { FolderOpen, FileArchive } from 'lucide-vue-next'
import { secondaryButton } from '@/features/configuration/style'
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
const feedback = shallowRef('')

async function run(action: 'logs' | 'data' | 'export') {
  feedback.value = ''
  try {
    if (action === 'export') {
      const result = await exportAction.mutateAsync()
      feedback.value = result.cancelled
        ? '已取消导出。'
        : `诊断包已保存为 ${result.filename}。分享前请检查包内内容。`
    } else {
      await (action === 'logs' ? logs.mutateAsync() : data.mutateAsync())
      feedback.value = action === 'logs' ? '已请求打开日志目录。' : '已请求打开数据目录。'
    }
  } catch {
    feedback.value =
      action === 'export'
        ? '诊断包导出失败或未通过安全检查，请查看本地日志后重试。'
        : '无法打开目录，请检查系统文件管理器和目录权限。'
  }
  if (action === 'export') void status.refetch()
}
</script>

<template>
  <section
    class="rounded-2xl border border-border-hairline bg-surface-solid p-5"
    aria-label="数据与日志"
  >
    <h2 class="mb-2 text-base font-semibold">数据与日志</h2>
    <div
      class="flex flex-wrap items-center justify-between gap-4 border-b border-border-hairline py-4"
    >
      <div>
        <strong class="text-sm">本地数据目录</strong>
        <p class="mt-1 text-xs text-text-tertiary">任务、设置和执行记录保存在本机 SQLite 中</p>
      </div>
      <button
        type="button"
        :class="secondaryButton"
        :disabled="data.isPending.value"
        @click="run('data')"
      >
        <FolderOpen class="size-4" aria-hidden="true" />打开数据目录
      </button>
    </div>
    <div class="flex flex-wrap items-center justify-between gap-4 py-4">
      <div class="min-w-0 flex-1">
        <strong class="text-sm">诊断日志目录</strong>
        <p class="mt-1 text-xs text-text-tertiary">
          日志与执行历史分别保存，即使暂无日志也可打开目录
        </p>
        <p v-if="status.data.value" class="mt-1 break-all text-xs text-text-tertiary">
          {{ status.data.value.directory }}
        </p>
      </div>
      <button
        type="button"
        :class="secondaryButton"
        :disabled="logs.isPending.value"
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
      @click="run('export')"
    >
      <FileArchive class="size-4" aria-hidden="true" />
      {{ exportAction.isPending.value ? '正在检查并导出…' : '导出诊断包…' }}
    </button>
    <p v-if="feedback" class="mt-3 text-sm text-text-secondary" role="status">{{ feedback }}</p>
  </section>
</template>
