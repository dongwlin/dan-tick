import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { Outcome } from '@bindings/github.com/dongwlin/dan-tick/internal/danmaku/models'
import { ListRooms } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import { TestSend } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { createAppQueryClient } from '@/app/queryClient'
import { mountToastHost } from '@/test/toastHost'
import TestSendPanel from './TestSendPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListRooms: vi.fn(),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  TestSend: vi.fn(),
}))

it('requires a saved room, distinguishes test send results, and never starts a task', async () => {
  const user = userEvent.setup()
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 7,
      title: '演示直播间',
      platformRoomId: '98765',
      anchorName: '',
      remark: '',
      coverUrl: '',
      taskCount: 0,
    },
  ])
  vi.mocked(TestSend)
    .mockResolvedValueOnce({ outcome: Outcome.Accepted, errorCode: '' })
    .mockResolvedValueOnce({ outcome: Outcome.Unknown, errorCode: 'send_failed' })
  mountToastHost()
  render(TestSendPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const card = screen.getByRole('region', { name: '测试发送' })
  expect(within(card).queryByText(/不写入任务执行历史/)).toBeNull()
  expect(within(card).queryByText(/与任务共用账号限流/)).toBeNull()
  const initialChildCount = card.children.length
  expect(await screen.findByRole('option', { name: '演示直播间' })).toBeTruthy()
  await user.selectOptions(screen.getByRole('combobox', { name: '目标直播间' }), '7')
  await user.type(screen.getByRole('textbox', { name: '测试弹幕内容' }), '测试文字')
  await user.click(screen.getByRole('button', { name: '发送测试弹幕' }))
  await waitFor(() =>
    expect(vi.mocked(TestSend)).toHaveBeenCalledWith({ roomId: 7, message: '测试文字' }),
  )
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '测试弹幕已被 B 站接受，已记入执行历史',
  )
  await user.click(screen.getByRole('button', { name: '发送测试弹幕' }))
  expect((await screen.findByRole('alert', { name: '操作警告通知' })).textContent).toContain(
    '发送结果无法确认，请勿立即重试；本次测试已记入执行历史',
  )
  expect(card.children).toHaveLength(initialChildCount)
  expect(within(card).queryByText(/发送结果无法确认/)).toBeNull()
})
