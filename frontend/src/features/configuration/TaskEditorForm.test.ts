import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import {
  CreateTask,
  GetTask,
  ListRooms,
  UpdateTask,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import { Kind as TaskKind } from '@bindings/github.com/dongwlin/dan-tick/internal/task/models'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import App from '@/app/App.vue'
import { createAppRouter } from '@/app/router'
import { createAppQueryClient } from '@/app/queryClient'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListRooms: vi.fn(),
  ListRoomPage: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
  ListTaskPage: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
  CreateTask: vi.fn(),
  GetTask: vi.fn(),
  UpdateTask: vi.fn(),
}))

it('validates task conditions, retains edits on failure and submits typed binding input', async () => {
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 1,
      platformRoomId: '98765',
      title: '测试直播间',
      anchorName: '主播',
      remark: '',
      taskCount: 0,
    },
  ])
  vi.mocked(CreateTask)
    .mockRejectedValueOnce(new Error('输入无效，请检查填写内容'))
    .mockResolvedValueOnce({
      id: 9,
      roomId: 1,
      name: '自动问候',
      kind: TaskKind.Single,
      messages: ['你好'],
      intervalSeconds: null,
      maxCount: null,
      maxDurationSeconds: null,
      schedule: {
        kind: ScheduleKind.Daily,
        timezone: 'Asia/Shanghai',
        localDate: '',
        localTime: '21:00:00',
        weekdays: 0,
      },
      enabled: true,
      nextDueAt: 12345,
      createdAt: 12345,
      updatedAt: 12345,
    })

  window.history.replaceState(null, '', '/#/tasks/new')
  const router = createAppRouter()
  const user = userEvent.setup()
  render(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  expect(await screen.findByRole('heading', { name: '新建任务' })).toBeTruthy()
  await screen.findByRole('option', { name: '测试直播间 · 98765' })
  await user.type(screen.getByRole('textbox', { name: '任务名称' }), '自动问候')
  await user.selectOptions(screen.getByRole('combobox', { name: '目标直播间' }), '1')
  await user.type(screen.getByRole('textbox', { name: '发送队列' }), '你好\n晚上好')
  await user.click(screen.getByRole('button', { name: '创建任务' }))
  expect(await screen.findByText('单次任务只能填写一条弹幕')).toBeTruthy()
  expect(vi.mocked(CreateTask)).not.toHaveBeenCalled()

  const messages = screen.getByRole<HTMLTextAreaElement>('textbox', { name: '发送队列' })
  await user.clear(messages)
  await user.type(messages, '你好')
  await user.click(screen.getByRole('button', { name: '创建任务' }))
  expect(await screen.findByText('输入无效，请检查填写内容')).toBeTruthy()
  expect(messages.value).toBe('你好')
  await user.click(screen.getByRole('button', { name: '创建任务' }))
  await waitFor(() => expect(window.location.hash).toBe('#/tasks'))
  expect(vi.mocked(CreateTask)).toHaveBeenLastCalledWith({
    roomId: 1,
    name: '自动问候',
    kind: TaskKind.Single,
    messages: ['你好'],
    intervalSeconds: null,
    maxCount: null,
    maxDurationSeconds: null,
    schedule: {
      kind: ScheduleKind.Daily,
      timezone: 'Asia/Shanghai',
      localDate: '',
      localTime: '21:00:00',
      weekdays: 0,
    },
    enabled: true,
  })
})

it('loads and edits a weekly loop task without losing message order', async () => {
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 1,
      platformRoomId: '98765',
      title: '测试直播间',
      anchorName: '',
      remark: '',
      taskCount: 1,
    },
  ])
  const saved = {
    id: 42,
    roomId: 1,
    name: '旧任务',
    kind: TaskKind.Loop,
    messages: ['第一条', '第二条'],
    intervalSeconds: 15,
    maxCount: 3,
    maxDurationSeconds: null,
    schedule: {
      kind: ScheduleKind.Weekly,
      timezone: 'Asia/Shanghai',
      localDate: '',
      localTime: '20:30:00',
      weekdays: 1 | (1 << 2),
    },
    enabled: true,
    nextDueAt: 12345,
    createdAt: 12345,
    updatedAt: 12345,
  }
  vi.mocked(GetTask).mockResolvedValue(saved)
  vi.mocked(UpdateTask).mockResolvedValue({ ...saved, name: '新任务' })
  window.history.replaceState(null, '', '/#/tasks/42/edit')
  const user = userEvent.setup()
  const router = createAppRouter()
  render(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const name = await screen.findByRole<HTMLInputElement>('textbox', { name: '任务名称' })
  expect(name.value).toBe('旧任务')
  expect(screen.getByRole<HTMLTextAreaElement>('textbox', { name: '发送队列' }).value).toBe(
    '第一条\n第二条',
  )
  expect(screen.getByRole<HTMLInputElement>('checkbox', { name: '周一' }).checked).toBe(true)
  expect(screen.getByRole<HTMLInputElement>('checkbox', { name: '周三' }).checked).toBe(true)
  await user.clear(name)
  await user.type(name, '新任务')
  await user.click(screen.getByRole('button', { name: '保存修改' }))
  await waitFor(() =>
    expect(vi.mocked(UpdateTask)).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        name: '新任务',
        kind: TaskKind.Loop,
        messages: ['第一条', '第二条'],
        intervalSeconds: 15,
        maxCount: 3,
        schedule: expect.objectContaining({ kind: ScheduleKind.Weekly, weekdays: 5 }),
      }),
    ),
  )
  await waitFor(() => expect(window.location.hash).toBe('#/tasks'))
})
