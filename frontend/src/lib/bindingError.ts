// Only messages from desktop.classify are safe to show. A runtime/network
// exception, including one containing arbitrary input, never reaches the UI.
const publicMessages = new Set([
  '操作失败，请重试或查看诊断日志',
  '修改已保存，但定时调度同步失败，请重新启动应用后检查',
  '输入无效，请检查填写内容',
  '弹幕内容超过本应用的长度上限，请缩短后重试',
  '该直播间已添加',
  '指定直播间或任务不存在',
  '目标直播间不存在或已删除',
  '存在正在执行的任务，请先停止执行',
  '应用正在关闭，暂时无法更改任务',
  '直播间验证暂不可用，请稍后再试',
  '操作已取消或超时',
  '系统安全凭证存储不可用，请检查系统密钥环',
  '应用正在关闭，暂时无法执行任务',
  '该执行已结束或不在当前进程中运行，请刷新状态',
  '指定执行记录不存在',
  '登录会话无法安全续期，请重新扫码登录',
  '二维码或登录会话已过期，请重新扫码',
  '登录服务暂不可用，请稍后重新扫码或检查登录状态',
  '应用正在关闭，暂时无法登录',
])

export function bindingError(error: unknown): string {
  if (!error || typeof error !== 'object') return '操作失败，请重试或查看诊断日志'
  const message = 'message' in error ? error.message : undefined
  return typeof message === 'string' && publicMessages.has(message)
    ? message
    : '操作失败，请重试或查看诊断日志'
}
