import { fireEvent, render, screen, within } from '@testing-library/vue'
import { expect, it } from 'vitest'
import { notify } from '@/composables/useToast'
import UiToastHost from './UiToastHost.vue'

it('keeps the toast live region in the document before any message', async () => {
  render(UiToastHost)

  const region = await screen.findByRole('region', { name: '通知 (F8)' })
  expect(within(region).getByRole('list')).toBeTruthy()
  expect(screen.queryByRole('status')).toBeNull()
  expect(screen.queryByRole('alert')).toBeNull()
})

it('shows one message at a time and dismisses it from the close button', async () => {
  render(UiToastHost)

  notify('测试弹幕已被 B 站接受，已记入执行历史。', 'success')
  const status = await screen.findByRole('status', { name: '操作成功通知' })
  expect(status.textContent).toContain('测试弹幕已被 B 站接受')

  notify('发送结果无法确认，请勿立即重试；本次测试已记入执行历史。', 'warning')
  expect((await screen.findByRole('alert', { name: '操作警告通知' })).textContent).toContain(
    '发送结果无法确认',
  )
  expect(screen.queryByRole('status')).toBeNull()

  await fireEvent.click(screen.getByRole('button', { name: '关闭通知' }))
  expect(screen.queryByRole('alert')).toBeNull()
})
