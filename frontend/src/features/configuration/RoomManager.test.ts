import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { Browser } from '@wailsio/runtime'
import { expect, it, vi } from 'vitest'
import {
  AddRoom,
  DeleteRoom,
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
  RefreshRoomMetadata: vi.fn().mockResolvedValue(false),
}))
vi.mock('@wailsio/runtime', () => ({ Browser: { OpenURL: vi.fn() } }))

function roomCover(title: string) {
  return screen.getByRole('heading', { name: title }).closest('article')?.firstElementChild
}

it('opens rooms in the system browser in both views and reports failures safely', async () => {
  vi.mocked(ListRoomPage).mockResolvedValue({
    items: [
      {
        id: 3,
        platformRoomId: '300',
        title: '直播三',
        anchorName: '主播',
        remark: '',
        coverUrl: '',
        taskCount: 0,
      },
    ],
    nextCursor: null,
  })
  vi.mocked(Browser.OpenURL)
    .mockReset()
    .mockResolvedValueOnce(undefined)
    .mockRejectedValueOnce(new Error('native browser error with private details'))
    .mockResolvedValueOnce(undefined)

  const user = userEvent.setup()
  render(RoomManager, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const link = await screen.findByRole<HTMLAnchorElement>('link', { name: '打开直播间 300' })
  expect(link.href).toBe('https://live.bilibili.com/300')
  const roomCard = screen.getByRole('heading', { name: '直播三' }).closest('article')
  expect(roomCard?.classList.contains('shadow-sm')).toBe(false)
  await user.click(link)
  await waitFor(() => expect(Browser.OpenURL).toHaveBeenCalledWith('https://live.bilibili.com/300'))

  await user.click(screen.getByRole('button', { name: '列表' }))
  expect(roomCard?.classList.contains('shadow-sm')).toBe(false)
  await user.click(screen.getByRole('link', { name: '打开直播间 300' }))
  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '无法打开直播间，请检查系统默认浏览器后重试',
  )
  expect(screen.getByRole('alert').textContent).not.toContain('private details')

  await user.click(screen.getByRole('link', { name: '打开直播间 300' }))
  await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  expect(Browser.OpenURL).toHaveBeenCalledTimes(3)
})

it('deletes a room only after the alert dialog confirms the consequence', async () => {
  vi.mocked(ListRoomPage).mockResolvedValue({
    items: [
      {
        id: 3,
        platformRoomId: '300',
        title: '直播三',
        anchorName: '',
        remark: '备注三',
        coverUrl: '',
        taskCount: 1,
      },
    ],
    nextCursor: null,
  })
  vi.mocked(DeleteRoom).mockResolvedValue(undefined)
  const user = userEvent.setup()
  render(RoomManager, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  const trigger = await screen.findByRole('button', { name: '删除直播间 300' })
  await user.click(trigger)
  const dialog = screen.getByRole('alertdialog', { name: '删除直播间' })
  expect(dialog.textContent).toContain('删除直播间“备注三”后')
  expect(dialog.textContent).toContain('关联任务会同时删除且无法恢复')
  await user.click(within(dialog).getByRole('button', { name: '取消' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(vi.mocked(DeleteRoom)).not.toHaveBeenCalled()

  await user.click(trigger)
  const confirmDialog = screen.getByRole('alertdialog', { name: '删除直播间' })
  await user.click(within(confirmDialog).getByRole('button', { name: '删除直播间' }))
  await waitFor(() => expect(vi.mocked(DeleteRoom)).toHaveBeenCalledWith(3))
})

it('loads real binding pages, switches views, adds rooms and retains input on failure', async () => {
  const rows = [
    {
      id: 3,
      platformRoomId: '300',
      title: '直播三',
      anchorName: '主播',
      remark: '',
      coverUrl: '',
      taskCount: 0,
    },
    {
      id: 2,
      platformRoomId: '200',
      title: '直播二',
      anchorName: '',
      remark: '直播二',
      coverUrl: '',
      taskCount: 1,
    },
    {
      id: 1,
      platformRoomId: '100',
      title: '直播一',
      anchorName: '',
      remark: '',
      coverUrl: '',
      taskCount: 0,
    },
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
  const grid = screen.getByRole('heading', { name: '直播三' }).closest('article')?.parentElement
  expect(grid?.classList.contains('grid-cols-1')).toBe(true)
  expect(grid?.classList.contains('min-[601px]:grid-cols-2')).toBe(true)
  expect(grid?.classList.contains('min-[1101px]:grid-cols-3')).toBe(true)
  expect(screen.getAllByText('直播三')).toHaveLength(1)
  expect(screen.getAllByText('直播二')).toHaveLength(1)
  expect(screen.getByText('主播 · 300')).toBeTruthy()
  const cover = roomCover('直播三')
  expect(cover?.classList.contains('h-[134px]')).toBe(true)
  expect(cover?.classList.contains('bg-(--accent-panel)')).toBe(true)
  const symbol = cover?.querySelector('[aria-hidden="true"]')
  expect(symbol?.classList.contains('size-[66px]')).toBe(true)
  expect(symbol?.querySelector('svg')?.classList.contains('size-[26px]')).toBe(true)
  expect(cover?.textContent?.trim()).toBe('')
  expect(roomCover('直播二')?.classList.contains('bg-surface-muted')).toBe(true)
  expect(screen.queryByRole('heading', { name: '直播一' })).toBeNull()
  await user.click(screen.getByRole('button', { name: '加载更多' }))
  expect(await screen.findByRole('heading', { name: '直播一' })).toBeTruthy()
  expect(roomCover('直播一')?.classList.contains('bg-(--accent-panel-alt)')).toBe(true)
  expect(vi.mocked(ListRoomPage)).toHaveBeenCalledWith({ cursor: 2, limit: 12 })
  await user.click(screen.getByRole('button', { name: '列表' }))
  expect(screen.getByRole('button', { name: '列表' }).getAttribute('aria-pressed')).toBe('true')
  expect(screen.getByText('主播：主播 · 300')).toBeTruthy()
  expect(grid?.classList.contains('flex')).toBe(true)
  expect(screen.getAllByText('直播二')).toHaveLength(1)
  const listActions = screen.getByRole('button', { name: '修改 300 的备注' }).parentElement
    ?.parentElement
  expect(listActions?.classList.contains('border-t')).toBe(false)
  expect(listActions?.classList.contains('md:ml-auto')).toBe(true)
  expect(listActions?.parentElement?.classList.contains('md:flex-row')).toBe(true)
  expect(listActions?.parentElement?.classList.contains('pr-7')).toBe(true)
  const actionControls = listActions?.querySelectorAll('a, button')
  expect(actionControls).toHaveLength(3)
  actionControls?.forEach((control) => {
    expect(control.classList.contains('size-11')).toBe(true)
    expect(control.classList.contains('justify-center')).toBe(true)
  })

  await user.click(screen.getByRole('button', { name: '卡片' }))
  const cardActions = screen.getByRole('button', { name: '修改 300 的备注' }).parentElement
    ?.parentElement
  expect(cardActions?.classList.contains('border-t')).toBe(true)
  expect(cardActions?.classList.contains('md:ml-auto')).toBe(false)
  expect(cardActions?.parentElement?.classList.contains('pr-7')).toBe(false)

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

it('shows a different room title or its missing-title hint without duplicating the primary label', async () => {
  vi.mocked(ListRoomPage).mockResolvedValue({
    items: [
      {
        id: 4,
        platformRoomId: '400',
        title: '原标题',
        anchorName: '主播',
        remark: '自定义备注',
        coverUrl: '',
        taskCount: 0,
      },
      {
        id: 5,
        platformRoomId: '500',
        title: '',
        anchorName: '',
        remark: '',
        coverUrl: '',
        taskCount: 0,
      },
      {
        id: 6,
        platformRoomId: '600',
        title: '相同标题',
        anchorName: '',
        remark: ' 相同标题 ',
        coverUrl: '',
        taskCount: 0,
      },
    ],
    nextCursor: null,
  })
  const user = userEvent.setup()
  render(RoomManager, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  expect(await screen.findByRole('heading', { name: '自定义备注' })).toBeTruthy()
  expect(screen.getByText('原标题')).toBeTruthy()
  expect(screen.getByRole('heading', { name: '房间 500' })).toBeTruthy()
  expect(screen.getByText('未获取到直播标题')).toBeTruthy()
  expect(screen.getAllByText(/相同标题/)).toHaveLength(1)
  await user.click(screen.getByRole('button', { name: '列表' }))
  expect(screen.getByText('原标题')).toBeTruthy()
  expect(screen.getAllByText(/相同标题/)).toHaveLength(1)
})

it('persists the card/list selection across unmounts and restores it on return', async () => {
  vi.mocked(ListRoomPage).mockResolvedValue({
    items: [
      {
        id: 3,
        platformRoomId: '300',
        title: '直播三',
        anchorName: '主播',
        remark: '',
        coverUrl: '',
        taskCount: 0,
      },
    ],
    nextCursor: null,
  })
  const user = userEvent.setup()
  const mount = () =>
    render(RoomManager, {
      global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
    })

  const first = mount()
  await screen.findByRole('heading', { name: '直播三' })
  expect(roomCover('直播三')?.classList.contains('h-[134px]')).toBe(true)
  await user.click(screen.getByRole('button', { name: '列表' }))
  expect(localStorage.getItem('dan-tick-room-view')).toBe('list')
  first.unmount()

  const second = mount()
  await screen.findByRole('heading', { name: '直播三' })
  expect(screen.getByRole('button', { name: '列表' }).getAttribute('aria-pressed')).toBe('true')
  expect(roomCover('直播三')?.classList.contains('h-[134px]')).toBe(false)
  await user.click(screen.getByRole('button', { name: '卡片' }))
  expect(localStorage.getItem('dan-tick-room-view')).toBe('cards')
  second.unmount()

  // Unknown persisted values must never break the default layout.
  localStorage.setItem('dan-tick-room-view', 'invalid')
  const third = mount()
  await screen.findByRole('heading', { name: '直播三' })
  expect(roomCover('直播三')?.classList.contains('h-[134px]')).toBe(true)
  expect(screen.getByRole('button', { name: '卡片' }).getAttribute('aria-pressed')).toBe('true')
  third.unmount()
})

it('continues switching views if localStorage is blocked', async () => {
  vi.mocked(ListRoomPage).mockResolvedValue({
    items: [
      {
        id: 3,
        platformRoomId: '300',
        title: '直播三',
        anchorName: '主播',
        remark: '',
        coverUrl: '',
        taskCount: 0,
      },
    ],
    nextCursor: null,
  })
  const denied = () => {
    throw new Error('storage unavailable')
  }
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(denied)
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(denied)

  const user = userEvent.setup()
  render(RoomManager, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  await screen.findByRole('heading', { name: '直播三' })
  expect(roomCover('直播三')?.classList.contains('h-[134px]')).toBe(true)
  await user.click(screen.getByRole('button', { name: '列表' }))
  expect(screen.getByRole('button', { name: '列表' }).getAttribute('aria-pressed')).toBe('true')
  expect(roomCover('直播三')?.classList.contains('h-[134px]')).toBe(false)
  await user.click(screen.getByRole('button', { name: '卡片' }))
  expect(screen.getByRole('button', { name: '卡片' }).getAttribute('aria-pressed')).toBe('true')
})
