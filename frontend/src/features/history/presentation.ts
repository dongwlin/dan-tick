import { executionStatus } from '@/features/operations/queries'

export function displayTime(value: number | null): string {
  return value === null ? '—' : new Date(value).toLocaleString('zh-CN', { hour12: false })
}

export function statusText(status: string): string {
  return executionStatus[status] ?? '状态未知'
}

export function triggerKindText(triggerKind: string): string {
  if (triggerKind === 'scheduled') return '计划执行'
  if (triggerKind === 'manual') return '手动执行'
  if (triggerKind === 'test') return '测试发送'
  return '未知来源'
}

export function statusClass(status: string): string {
  if (status === 'completed') return 'bg-soft-success-bg text-soft-success-fg'
  if (['failed', 'interrupted'].includes(status)) return 'bg-soft-danger-bg text-soft-danger-fg'
  if (['missed', 'stopped'].includes(status)) return 'bg-soft-warning-bg text-soft-warning-fg'
  return 'bg-soft-accent-bg text-accent-text'
}

const errorDescriptions: Record<string, string> = {
  credential_missing: '尚未配置账号凭证',
  credential_unavailable: '系统密钥环不可用',
  credential_corrupt: '账号凭证无法读取',
  authentication: '登录已失效',
  rate_limited: '触发平台发送频率限制',
  bad_request: '平台拒绝了发送参数',
  too_long: '弹幕超过长度限制',
  rejected: '平台拒绝发送',
  invalid_input: '发送内容无效',
  timeout: '请求超时，结果可能不确定',
  cancelled: '请求已取消',
  transport: '网络连接失败',
  http_status: '平台服务响应异常',
  invalid_response: '无法识别平台响应',
  uncertain: '发送结果无法确认，请勿立即重发',
  send_failure: '发送失败',
  persistence: '执行记录保存失败',
  wait_failure: '发送等待失败',
  user_stopped: '用户主动停止执行',
  task_busy: '同一任务已有活跃执行',
  schedule_overdue: '已错过计划执行时间',
  recovery_unknown: '恢复时无法确认发送结果',
  recovery_interrupted: '应用重启前执行未完成',
  handoff_failed: '调度交接失败',
}

export function errorText(code: string | null): string {
  if (!code) return '未记录异常'
  return errorDescriptions[code] ?? '未知错误，可查看诊断日志'
}

export function attemptResult(result: string): string {
  const labels: Record<string, string> = {
    pending: '待确认',
    accepted: '发送成功',
    failed: '发送失败',
    unknown: '结果不确定',
    not_sent: '未发送',
  }
  return labels[result] ?? '未知结果'
}
