import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import {
  CheckAccount,
  ClearAccount,
  LatestExecutions,
  StartManual,
  StopExecution,
  TestSend,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import type { TestSendInput } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'

export const operationKeys = {
  account: ['account', 'status'] as const,
  executions: ['executions', 'latest'] as const,
}

export function useAccountStatus() {
  return useQuery({ queryKey: operationKeys.account, queryFn: CheckAccount })
}

export function useLatestExecutions() {
  return useQuery({ queryKey: operationKeys.executions, queryFn: LatestExecutions })
}

export function useClearAccount() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ClearAccount,
    onSettled: () => client.invalidateQueries({ queryKey: operationKeys.account }),
  })
}

export function useTestSend() {
  return useMutation({ mutationFn: (input: TestSendInput) => TestSend(input) })
}

export function useStartManual() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (taskId: number) => StartManual(taskId),
    onSettled: () => client.invalidateQueries({ queryKey: operationKeys.executions }),
  })
}

export function useStopExecution() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (executionId: number) => StopExecution(executionId),
    onSettled: () => client.invalidateQueries({ queryKey: operationKeys.executions }),
  })
}

export const executionStatus: Record<string, string> = {
  waiting: '等待执行',
  queued: '排队中',
  running: '正在执行',
  completed: '已完成',
  stopped: '已停止',
  failed: '执行失败',
  missed: '已错过',
  interrupted: '执行中断',
}

export const sendFailureText: Record<string, string> = {
  credential_missing: '尚未配置账号凭证',
  credential_unavailable: '系统安全凭证存储不可用，请检查系统密钥环',
  authentication: '登录已失效，请更新账号凭证',
  rate_limited: '触发 B 站发送频率限制，请稍后再试',
  too_long: '弹幕内容超出平台长度限制',
  invalid_input: '发送内容无效，请检查输入',
  timeout: '请求超时，发送结果可能无法确认，请勿立即重试',
  cancelled: '发送操作已取消',
  send_failed: '发送失败或状态无法确认，请查看诊断日志',
}
