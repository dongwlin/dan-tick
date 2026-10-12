import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import {
  DeleteTask,
  ListRooms,
  ListTaskPage,
  SetTaskEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import { Kind as TaskKind } from '@bindings/github.com/dongwlin/dan-tick/internal/task/models'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import { createAppRouter } from '@/app/router'
import { createAppQueryClient } from '@/app/queryClient'
import type { Room, Task } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import TaskManager from './TaskManager.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListTaskPage: vi.fn(),
  ListRooms: vi.fn(),
  SetTaskEnabled: vi.fn(),
  DeleteTask: vi.fn(),
}))

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  LatestExecutions: vi.fn().mockResolvedValue([]),
  StartManual: vi.fn(),
  StopExecution: vi.fn(),
}))

const rooms: Room[] = [
  {
    id: 1,
    platformRoomId: '1001',
    title: '星河放映室',
    anchorName: '星河',
    coverUrl: '',
    remark: '',
    taskCount: 1,
  },
  {
    id: 2,
    platformRoomId: '1002',
    title: '小岛的直播间',
    anchorName: '小岛',
    coverUrl: '',
    remark: '',
    taskCount: 1,
  },
]

beforeEach(() => {
  vi.mocked(ListRooms).mockResolvedValue(rooms)
})

it('loads task pages, resets cursor on search and room changes, and applies enable changes', async () => {
  const items: Task[] = [
    {
      id: 3,
      roomId: 1,
      name: '第三个任务',
      kind: TaskKind.Single,
      enabled: true,
      messages: ['你好'],
      intervalSeconds: null,
      maxCount: null,
      maxDurationSeconds: null,
      schedule: {
        kind: ScheduleKind.Daily,
        timezone: 'Asia/Shanghai',
        localDate: '',
        localTime: '09:00:00',
        weekdays: 0,
      },
      nextDueAt: 12345,
      createdAt: 1,
      updatedAt: 1,
    },
    {
      id: 2,
      roomId: 2,
      name: '另一个任务',
      kind: TaskKind.Single,
      enabled: true,
      messages: ['你好'],
      intervalSeconds: null,
      maxCount: null,
      maxDurationSeconds: null,
      schedule: {
        kind: ScheduleKind.Daily,
        timezone: 'Asia/Shanghai',
        localDate: '',
        localTime: '09:00:00',
        weekdays: 0,
      },
      nextDueAt: 12345,
      createdAt: 1,
      updatedAt: 1,
    },
    {
      id: 1,
      roomId: 1,
      name: '初始任务',
      kind: TaskKind.Single,
      enabled: false,
      messages: ['你好'],
      intervalSeconds: null,
      maxCount: null,
      maxDurationSeconds: null,
      schedule: {
        kind: ScheduleKind.Daily,
        timezone: 'Asia/Shanghai',
        localDate: '',
        localTime: '09:00:00',
        weekdays: 0,
      },
      nextDueAt: null,
      createdAt: 1,
      updatedAt: 1,
    },
  ]
  const first = items[0]
  if (!first) throw new Error('missing test task')
  vi.mocked(ListTaskPage).mockImplementation(({ search, cursor, roomId }) => {
    const matching = items.filter(
      (item) => (!search || item.name.includes(search)) && (!roomId || item.roomId === roomId),
    )
    const page = matching.filter((item) => cursor === null || item.id < cursor)
    return Promise.resolve({
      items: page.slice(0, 2),
      nextCursor: page.length > 2 ? (page[1]?.id ?? null) : null,
    }) as ReturnType<typeof ListTaskPage>
  })
  vi.mocked(SetTaskEnabled).mockResolvedValue({ ...first, enabled: false })
  window.history.replaceState(null, '', '/#/tasks')
  const router = createAppRouter()
  const user = userEvent.setup()
  render(TaskManager, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  expect(await screen.findByRole('heading', { name: '第三个任务' })).toBeTruthy()
  const listPanel = screen.getByRole('heading', { name: '我的任务' }).parentElement?.parentElement
  expect(listPanel?.classList.contains('acrylic')).toBe(true)
  expect(listPanel?.classList.contains('bg-surface-solid')).toBe(false)
  expect(listPanel?.classList.contains('shadow-sm')).toBe(false)
  expect(screen.queryByRole('heading', { name: '初始任务' })).toBeNull()
  await user.click(screen.getByRole('button', { name: '加载更多' }))
  expect(await screen.findByRole('heading', { name: '初始任务' })).toBeTruthy()

  await user.type(screen.getByRole('searchbox', { name: '搜索任务或直播间' }), '第三')
  await waitFor(() =>
    expect(vi.mocked(ListTaskPage)).toHaveBeenLastCalledWith({
      cursor: null,
      limit: 12,
      roomId: 0,
      search: '第三',
    }),
  )
  await waitFor(() => expect(screen.queryByRole('heading', { name: '初始任务' })).toBeNull())
  expect(screen.queryByText('自动执行开启')).toBeNull()
  expect(screen.queryByText('已启用')).toBeNull()
  expect(screen.getAllByText('下次执行')).toHaveLength(1)
  expect(screen.getAllByText('单次发送 · 1 条消息')).toHaveLength(1)
  expect(screen.getByText(/星河放映室/)).toBeTruthy()
  expect(screen.getByText(/每天 09:00/)).toBeTruthy()
  expect(screen.getByRole('button', { name: '立即执行 第三个任务' })).toBeTruthy()
  expect(screen.getByRole('link', { name: '编辑任务 第三个任务' })).toBeTruthy()
  expect(screen.getByRole('button', { name: '删除任务 第三个任务' })).toBeTruthy()
  await user.click(screen.getByRole('switch', { name: '第三个任务 自动执行' }))
  await waitFor(() => expect(vi.mocked(SetTaskEnabled)).toHaveBeenCalledWith(3, false))

  await router.push('/tasks?room=2')
  await waitFor(() =>
    expect(vi.mocked(ListTaskPage)).toHaveBeenLastCalledWith({
      cursor: null,
      limit: 12,
      roomId: 2,
      search: '第三',
    }),
  )
  expect(screen.queryByRole('heading', { name: '第三个任务' })).toBeNull()
})

it('centers the empty task state without a duplicate create button and retains clear search', async () => {
  vi.mocked(ListTaskPage).mockResolvedValue({ items: [], nextCursor: null })
  window.history.replaceState(null, '', '/#/tasks')
  const user = userEvent.setup()
  render(TaskManager, {
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })
  const emptyState = await screen.findByText('还没有创建弹幕任务')
  expect(emptyState.parentElement?.classList.contains('justify-center')).toBe(true)
  expect(emptyState.parentElement?.querySelector('svg')).not.toBeNull()
  expect(screen.getByLabelText('弹幕任务列表').classList.contains('flex')).toBe(true)
  expect(screen.getAllByRole('link', { name: '新建任务' })).toHaveLength(1)
  await user.type(screen.getByRole('searchbox', { name: '搜索任务或直播间' }), '无结果')
  expect(await screen.findByText('没有找到匹配的任务')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: '清除搜索' }))
  expect(await screen.findByText('还没有创建弹幕任务')).toBeTruthy()
})

it('deletes a task only after the alert dialog confirms the consequence', async () => {
  const task: Task = {
    id: 3,
    roomId: 1,
    name: '第三个任务',
    kind: TaskKind.Single,
    enabled: true,
    messages: ['你好'],
    intervalSeconds: null,
    maxCount: null,
    maxDurationSeconds: null,
    schedule: {
      kind: ScheduleKind.Daily,
      timezone: 'Asia/Shanghai',
      localDate: '',
      localTime: '09:00:00',
      weekdays: 0,
    },
    nextDueAt: 12345,
    createdAt: 1,
    updatedAt: 1,
  }
  vi.mocked(ListTaskPage).mockResolvedValue({ items: [task], nextCursor: null })
  vi.mocked(DeleteTask).mockResolvedValue(undefined)
  window.history.replaceState(null, '', '/#/tasks')
  const user = userEvent.setup()
  render(TaskManager, {
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })

  const trigger = await screen.findByRole('button', { name: '删除任务 第三个任务' })
  await user.click(trigger)
  const dialog = screen.getByRole('alertdialog', { name: '删除任务' })
  expect(dialog.textContent).toContain('删除任务“第三个任务”后无法恢复')
  expect(dialog.textContent).toContain('已保存的执行历史不会删除')
  await user.click(within(dialog).getByRole('button', { name: '取消' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(vi.mocked(DeleteTask)).not.toHaveBeenCalled()

  await user.click(trigger)
  const confirmDialog = screen.getByRole('alertdialog', { name: '删除任务' })
  await user.click(within(confirmDialog).getByRole('button', { name: '删除任务' }))
  await waitFor(() => expect(vi.mocked(DeleteTask)).toHaveBeenCalledWith(3))
})
