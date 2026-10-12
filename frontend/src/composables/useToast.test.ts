import { expect, it } from 'vitest'
import { clearToasts, dismissToast, notify, useToast } from './useToast'

function shownToast() {
  const { toast } = useToast()
  if (!toast.value) throw new Error('Expected a toast to be shown')
  return toast.value
}

it('keeps one message and replaces it with the next notification', () => {
  const { toast } = useToast()
  expect(toast.value).toBeNull()

  notify('默认时区已保存，仅应用于新建任务。')
  expect(shownToast()).toMatchObject({
    message: '默认时区已保存，仅应用于新建任务。',
    tone: 'success',
  })

  const firstId = shownToast().id
  notify('保存窗口偏好失败，设置未更改。', 'error')
  expect(shownToast().tone).toBe('error')
  expect(shownToast().id).not.toBe(firstId)
})

it('dismisses the shown message and ignores a late close from a replaced one', () => {
  notify('第一条消息。')
  const staleId = shownToast().id
  notify('第二条消息。', 'warning')

  dismissToast(staleId)
  expect(shownToast().message).toBe('第二条消息。')

  dismissToast(shownToast().id)
  expect(useToast().toast.value).toBeNull()
})

it('clears the shared slot without an id', () => {
  notify('诊断包已保存。', 'info')
  dismissToast()
  expect(useToast().toast.value).toBeNull()

  notify('已取消导出。', 'info')
  clearToasts()
  expect(useToast().toast.value).toBeNull()
})
