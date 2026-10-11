import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import {
  ListTaskPage,
  SetTaskEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import { Kind as TaskKind } from '@bindings/github.com/dongwlin/dan-tick/internal/task/models'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import { createAppRouter } from '@/app/router'
import { createAppQueryClient } from '@/app/queryClient'
import type { Task } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import TaskManager from './TaskManager.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListTaskPage: vi.fn(),
  SetTaskEnabled: vi.fn(),
  DeleteTask: vi.fn(),
}))

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
  expect(screen.queryByRole('heading', { name: '初始任务' })).toBeNull()
  await user.click(screen.getByRole('checkbox', { name: '第三个任务 自动执行' }))
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
