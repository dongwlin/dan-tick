import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import {
  AddRoom,
  ListRoomPage,
  UpdateRoom,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import { createAppQueryClient } from '@/app/queryClient'
import RoomManager from './RoomManager.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListRoomPage: vi.fn(),
  AddRoom: vi.fn(),
  UpdateRoom: vi.fn(),
  DeleteRoom: vi.fn(),
}))

it('loads real binding pages, switches views, adds rooms and retains input on failure', async () => {
  const rows = [
    { id: 3, platformRoomId: '300', title: '直播三', anchorName: '主播', remark: '', taskCount: 0 },
    { id: 2, platformRoomId: '200', title: '直播二', anchorName: '', remark: '', taskCount: 1 },
    { id: 1, platformRoomId: '100', title: '直播一', anchorName: '', remark: '', taskCount: 0 },
  ]
  const first = rows[0]
  if (!first) throw new Error('missing test room')
  vi.mocked(ListRoomPage).mockImplementation(
    ({ cursor }) =>
      Promise.resolve(
        cursor === null
          ? { items: rows.slice(0, 2), nextCursor: 2 }
          : { items: rows.slice(2), nextCursor: null },
      ) as ReturnType<typeof ListRoomPage>,
  )
  vi.mocked(AddRoom).mockRejectedValueOnce(new Error('该直播间已添加')).mockResolvedValueOnce(first)
  vi.mocked(UpdateRoom).mockResolvedValue({ ...first, remark: '备注' })

  const user = userEvent.setup()
  render(RoomManager, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  expect(await screen.findByRole('heading', { name: '直播三' })).toBeTruthy()
  const cover = screen.getByText('直播间 · 300').parentElement
  expect(cover?.classList.contains('h-[134px]')).toBe(true)
  expect(cover?.classList.contains('bg-(--accent-panel)')).toBe(true)
  const symbol = cover?.querySelector('[aria-hidden="true"]')
  expect(symbol?.classList.contains('size-[66px]')).toBe(true)
  expect(symbol?.querySelector('svg')?.classList.contains('size-[26px]')).toBe(true)
  expect(
    screen.getByText('直播间 · 200').parentElement?.classList.contains('bg-surface-muted'),
  ).toBe(true)
  expect(screen.queryByRole('heading', { name: '直播一' })).toBeNull()
  await user.click(screen.getByRole('button', { name: '加载更多' }))
  expect(await screen.findByRole('heading', { name: '直播一' })).toBeTruthy()
  expect(
    screen.getByText('直播间 · 100').parentElement?.classList.contains('bg-(--accent-panel-alt)'),
  ).toBe(true)
  expect(vi.mocked(ListRoomPage)).toHaveBeenCalledWith({ cursor: 2, limit: 12 })
  await user.click(screen.getByRole('button', { name: '列表' }))
  expect(screen.getByRole('button', { name: '列表' }).getAttribute('aria-pressed')).toBe('true')

  await user.click(screen.getByRole('button', { name: '添加直播间' }))
  const input = screen.getByRole<HTMLInputElement>('textbox', { name: '直播间 ID 或链接' })
  await user.type(input, '12345')
  await user.click(screen.getByRole('button', { name: '添加到列表' }))
  expect(await screen.findByRole('alert')).toHaveProperty('textContent', '该直播间已添加')
  expect(input.value).toBe('12345')
  await user.click(screen.getByRole('button', { name: '添加到列表' }))
  await waitFor(() => expect(vi.mocked(AddRoom)).toHaveBeenCalledTimes(2))
  expect(screen.queryByRole('textbox', { name: '直播间 ID 或链接' })).toBeNull()

  await user.click(screen.getByRole('button', { name: '修改 300 的备注' }))
  const form = screen.getByRole<HTMLInputElement>('textbox', { name: '备注名称' })
  await user.type(form, '备注')
  await user.click(screen.getByRole('button', { name: '保存备注' }))
  await waitFor(() => expect(vi.mocked(UpdateRoom)).toHaveBeenCalledWith(3, { remark: '备注' }))
})
