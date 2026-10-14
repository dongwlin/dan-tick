import { VueQueryPlugin } from '@tanstack/vue-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/vue'
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

type User = ReturnType<typeof userEvent.setup>

/** Opens the room picker; finding the option also waits for the room read. */
async function pickRoom(user: User, label: string) {
  await user.click(screen.getByRole('combobox', { name: '目标直播间' }))
  const listbox = await screen.findByRole('listbox')
  await user.click(await within(listbox).findByRole('option', { name: label }))
}

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
  await pickRoom(user, '演示直播间')
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

it('validates programmatic submissions, focuses the invalid field, and sends only corrected input', async () => {
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
  vi.mocked(TestSend).mockResolvedValue({ outcome: Outcome.Accepted, errorCode: '' })
  const user = userEvent.setup()
  mountToastHost()
  render(TestSendPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const room = screen.getByRole('combobox', { name: '目标直播间' })
  const message = screen.getByRole<HTMLTextAreaElement>('textbox', { name: '测试弹幕内容' })
  const form = message.closest('form')
  if (!form) throw new Error('Expected the test send form to be present')
  expect(message.hasAttribute('maxlength')).toBe(false)
  await fireEvent.submit(form)
  expect(await screen.findByText('请选择直播间')).toBeTruthy()
  await waitFor(() => expect(document.activeElement).toBe(room))
  expect(vi.mocked(TestSend)).not.toHaveBeenCalled()

  await pickRoom(user, '演示直播间')
  for (const invalid of [
    '',
    '   ',
    '中'.repeat(41),
    'a'.repeat(81),
    '😀'.repeat(21),
    '文\u0000字',
  ]) {
    await fireEvent.update(message, invalid)
    await fireEvent.submit(form)
    await waitFor(() => expect(message.getAttribute('aria-invalid')).toBe('true'))
    await waitFor(() => expect(document.activeElement).toBe(message))
    expect(message.value).toBe(invalid)
    expect(message.getAttribute('aria-describedby')).toBe(
      'test-message-error test-message-description',
    )
    expect(vi.mocked(TestSend)).not.toHaveBeenCalled()
  }
  expect(screen.getByText('弹幕内容含有无效字符，请移除后重试')).toBeTruthy()

  await fireEvent.update(message, '😀'.repeat(20))
  await waitFor(() => expect(message.hasAttribute('aria-invalid')).toBe(false))
  await user.click(screen.getByRole('button', { name: '发送测试弹幕' }))
  await waitFor(() =>
    expect(vi.mocked(TestSend)).toHaveBeenCalledExactlyOnceWith({
      roomId: 7,
      message: '😀'.repeat(20),
    }),
  )
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '测试弹幕已被 B 站接受',
  )
})

it('keeps the original message after a backend failure', async () => {
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
  vi.mocked(TestSend).mockRejectedValue(new Error('弹幕内容超过本应用的长度上限，请缩短后重试'))
  const user = userEvent.setup()
  mountToastHost()
  render(TestSendPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  await pickRoom(user, '演示直播间')
  const message = screen.getByRole<HTMLTextAreaElement>('textbox', { name: '测试弹幕内容' })
  await user.type(message, '  原文保留  ')
  await user.click(screen.getByRole('button', { name: '发送测试弹幕' }))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '弹幕内容超过本应用的长度上限，请缩短后重试',
  )
  expect(vi.mocked(TestSend)).toHaveBeenCalledWith({ roomId: 7, message: '  原文保留  ' })
  expect(message.value).toBe('  原文保留  ')
})
