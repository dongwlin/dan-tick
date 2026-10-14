import { VueQueryPlugin } from '@tanstack/vue-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/vue'
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
import { GetPreferences } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice'
import App from '@/app/App.vue'
import { createAppRouter } from '@/app/router'
import { createAppQueryClient } from '@/app/queryClient'
import { timezoneOptions } from '@/lib/timezones'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListRooms: vi.fn(),
  ListRoomPage: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
  ListTaskPage: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
  CreateTask: vi.fn(),
  GetTask: vi.fn(),
  UpdateTask: vi.fn(),
}))

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice', () => ({
  GetPreferences: vi
    .fn()
    .mockResolvedValue({ defaultTimezone: 'Asia/Shanghai', schedulingEnabled: true }),
}))

function requiredElement<T extends Element>(element: T | null): T {
  if (!element) throw new Error('Expected a form element to be present')
  return element
}

it('validates task conditions, retains edits on failure and submits typed binding input', async () => {
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 1,
      platformRoomId: '98765',
      title: '测试直播间',
      anchorName: '主播',
      remark: '',
      coverUrl: '',
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
  for (const name of ['基本信息', '弹幕内容', '执行计划', '执行预览']) {
    const card = screen.getByRole('heading', { name }).closest('section')
    expect(card?.classList.contains('shadow-sm')).toBe(false)
  }
  for (const description of [
    '先决定把什么内容发到哪里',
    '单次发送一条弹幕',
    '指定触发方式和时间',
  ]) {
    expect(screen.queryByText(description)).toBeNull()
  }
  await screen.findByRole('option', { name: '测试直播间 · 98765' })
  const messageCard = requiredElement(
    screen.getByRole('heading', { name: '弹幕内容' }).closest('section'),
  )
  const cardChildren = messageCard.children.length
  const errorSlot = requiredElement(document.getElementById('task-messages-error'))
  const actions = requiredElement(screen.getByRole('button', { name: '创建任务' }).closest('aside'))
  const actionChildren = actions.children.length
  expect(errorSlot.textContent).toBe('')
  expect(errorSlot.previousElementSibling?.textContent).toBe('发送队列')
  expect(errorSlot.parentElement?.nextElementSibling).toBe(
    screen.getByRole('textbox', { name: '发送队列' }),
  )
  expect(screen.getByLabelText('任务名称')).toBe(screen.getByRole('textbox', { name: '任务名称' }))
  const messagesInput = screen.getByRole<HTMLTextAreaElement>('textbox', { name: '发送队列' })
  expect(messagesInput.placeholder).toBe('在这里输入弹幕内容…')
  expect(messagesInput.getAttribute('aria-describedby')).toBe(
    'task-messages-error task-messages-description',
  )
  expect(
    screen.getByText(
      '每行一条，空行自动忽略；本应用上限相当于 20 个英文字母、10 个普通汉字或 5 个 😀 表情；混合内容按比例计数，组合表情可能占更多长度。实际发送仍受平台规则限制。',
    ).id,
  ).toBe('task-messages-description')
  expect(messageCard.querySelector('p')).toBeNull()
  expect(screen.queryByText('执行计划使用明确时区，保存后由后端计算下次执行时间。')).toBeNull()
  await user.type(screen.getByRole('textbox', { name: '任务名称' }), '自动问候')
  await user.selectOptions(screen.getByRole('combobox', { name: '目标直播间' }), '1')
  await user.type(screen.getByRole('textbox', { name: '发送队列' }), '你好\n晚上好')
  await user.click(screen.getByRole('button', { name: '创建任务' }))
  expect(await screen.findByText('单次任务只能填写一条弹幕')).toBeTruthy()
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '表单校验未通过，请根据字段提示修改后重试。',
  )
  expect(screen.getByRole('textbox', { name: '发送队列' }).getAttribute('aria-invalid')).toBe(
    'true',
  )
  expect(messageCard.children).toHaveLength(cardChildren)
  expect(document.getElementById('task-messages-error')).toBe(errorSlot)
  expect(errorSlot.parentElement?.nextElementSibling).toBe(
    screen.getByRole('textbox', { name: '发送队列' }),
  )
  expect(actions.children).toHaveLength(actionChildren)
  expect(vi.mocked(CreateTask)).not.toHaveBeenCalled()

  const messages = screen.getByRole<HTMLTextAreaElement>('textbox', { name: '发送队列' })
  await user.clear(messages)
  await user.type(messages, '你好')
  await waitFor(() => expect(errorSlot.textContent).toBe(''))
  await user.click(screen.getByRole('button', { name: '创建任务' }))
  await waitFor(() =>
    expect(screen.getByRole('alert', { name: '操作失败通知' }).textContent).toContain(
      '输入无效，请检查填写内容',
    ),
  )
  expect(actions.children).toHaveLength(actionChildren)
  expect(within(actions).queryByText('输入无效，请检查填写内容')).toBeNull()
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

it('shows validation summary in a toast while keeping field errors and focusing the first field', async () => {
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 2,
      platformRoomId: '98765',
      title: '测试直播间',
      anchorName: '',
      remark: '',
      coverUrl: '',
      taskCount: 0,
    },
  ])
  window.history.replaceState(null, '', '/#/tasks/new')
  const user = userEvent.setup()
  render(App, {
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })
  await screen.findByRole('option', { name: '测试直播间 · 98765' })
  const nameError = requiredElement(document.getElementById('task-name-error'))
  const roomError = requiredElement(document.getElementById('task-roomId-error'))
  const messageError = requiredElement(document.getElementById('task-messages-error'))
  const header = requiredElement(
    screen.getByRole('heading', { name: '新建任务' }).closest('header'),
  )
  expect(header.textContent).toBe('新建任务')
  await user.click(screen.getByRole('button', { name: '创建任务' }))
  const toast = await screen.findByRole('alert', { name: '操作失败通知' })
  expect(toast.textContent).toContain('表单校验未通过，请根据字段提示修改后重试。')
  expect(toast.closest('form')).toBeNull()
  expect(header.textContent).toBe('新建任务')
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByRole('textbox', { name: '任务名称' })),
  )
  expect(nameError.textContent).toBe('请输入任务名称')
  expect(roomError.textContent).toBe('请选择直播间')
  expect(messageError.textContent).toBe('请填写至少一条弹幕')
  expect(document.getElementById('task-name-error')).toBe(nameError)
  expect(document.getElementById('task-roomId-error')).toBe(roomError)
  expect(document.getElementById('task-messages-error')).toBe(messageError)
  expect(vi.mocked(CreateTask)).not.toHaveBeenCalled()
})

it.each(['single', 'loop'])(
  'validates each cleaned message of a %s task and preserves invalid drafts until corrected',
  async (kind) => {
    vi.mocked(ListRooms).mockResolvedValue([
      {
        id: 1,
        platformRoomId: '98765',
        title: '测试直播间',
        anchorName: '',
        remark: '',
        coverUrl: '',
        taskCount: 0,
      },
    ])
    vi.mocked(CreateTask).mockResolvedValue({
      id: 9,
      roomId: 1,
      name: '弹幕校验',
      kind: kind === 'loop' ? TaskKind.Loop : TaskKind.Single,
      messages: [],
      intervalSeconds: kind === 'loop' ? 15 : null,
      maxCount: kind === 'loop' ? 1 : null,
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
    const user = userEvent.setup()
    render(App, {
      global: {
        plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
      },
    })
    await screen.findByRole('option', { name: '测试直播间 · 98765' })
    await user.type(screen.getByRole('textbox', { name: '任务名称' }), '弹幕校验')
    await user.selectOptions(screen.getByRole('combobox', { name: '目标直播间' }), '1')
    await user.selectOptions(screen.getByRole('combobox', { name: '任务类型' }), kind)
    const messages = screen.getByRole<HTMLTextAreaElement>('textbox', { name: '发送队列' })
    const validFirst = kind === 'loop' ? `  ${'中'.repeat(10)}  \n\n` : ''
    const invalidDraft = `${validFirst}  ${'a'.repeat(21)}  `
    await fireEvent.update(messages, invalidDraft)
    await user.click(screen.getByRole('button', { name: '创建任务' }))

    expect(
      await screen.findByText(
        `第 ${kind === 'loop' ? 2 : 1} 条：弹幕内容超过本应用的长度上限，请缩短后重试`,
      ),
    ).toBeTruthy()
    await waitFor(() => expect(document.activeElement).toBe(messages))
    expect(messages.value).toBe(invalidDraft)
    expect(messages.hasAttribute('maxlength')).toBe(false)
    expect(vi.mocked(CreateTask)).not.toHaveBeenCalled()

    await fireEvent.update(messages, `${validFirst}文\u0000字`)
    await user.click(screen.getByRole('button', { name: '创建任务' }))
    expect(
      await screen.findByText(
        `第 ${kind === 'loop' ? 2 : 1} 条：弹幕内容含有无效字符，请移除后重试`,
      ),
    ).toBeTruthy()
    expect(vi.mocked(CreateTask)).not.toHaveBeenCalled()

    await fireEvent.update(messages, `${validFirst}  ${'😀'.repeat(5)}  `)
    await waitFor(() => expect(messages.hasAttribute('aria-invalid')).toBe(false))
    await user.click(screen.getByRole('button', { name: '创建任务' }))
    await waitFor(() => expect(window.location.hash).toBe('#/tasks'))
    expect(vi.mocked(CreateTask)).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({
        messages: kind === 'loop' ? ['中'.repeat(10), '😀'.repeat(5)] : ['😀'.repeat(5)],
      }),
    )
  },
)

it('reuses the room selector position for loading failures and empty-room recovery', async () => {
  vi.mocked(ListRooms).mockRejectedValueOnce(new Error('读取失败')).mockResolvedValueOnce([])
  window.history.replaceState(null, '', '/#/tasks/new')
  const user = userEvent.setup()
  render(App, {
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })
  const retry = await screen.findByRole('button', { name: /重试读取直播间/ })
  const roomField = requiredElement(retry.parentElement)
  const childrenCount = roomField.children.length
  expect(screen.queryByRole('link', { name: /先添加直播间/ })).toBeNull()
  await user.click(retry)
  const addRoom = await screen.findByRole('link', { name: '暂无直播间，先添加直播间' })
  expect(addRoom.parentElement).toBe(roomField)
  expect(roomField.children).toHaveLength(childrenCount)
  expect(screen.queryByRole('button', { name: /重试读取直播间/ })).toBeNull()
  expect(screen.getByRole('button', { name: '创建任务' }).hasAttribute('disabled')).toBe(true)
})

it('loads and edits a weekly loop task without losing message order', async () => {
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 1,
      platformRoomId: '98765',
      title: '测试直播间',
      anchorName: '',
      remark: '',
      coverUrl: '',
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
  vi.mocked(UpdateTask)
    .mockRejectedValueOnce(new Error('修改已保存，但定时调度同步失败，请重新启动应用后检查'))
    .mockResolvedValueOnce({ ...saved, name: '新任务' })
  window.history.replaceState(null, '', '/#/tasks/42/edit')
  const user = userEvent.setup()
  const router = createAppRouter()
  render(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const name = await screen.findByRole<HTMLInputElement>('textbox', { name: '任务名称' })
  expect(name.value).toBe('旧任务')
  expect(screen.queryByText('每行一条，按照顺序依次发送，到末尾后循环')).toBeNull()
  expect(screen.getByRole<HTMLTextAreaElement>('textbox', { name: '发送队列' }).value).toBe(
    '第一条\n第二条',
  )
  const monday = screen.getByRole('checkbox', { name: '周一' })
  const tuesday = screen.getByRole('checkbox', { name: '周二' })
  expect(monday.getAttribute('aria-checked')).toBe('true')
  expect(screen.getByRole('checkbox', { name: '周三' }).getAttribute('aria-checked')).toBe('true')
  expect(tuesday.getAttribute('aria-checked')).toBe('false')
  await user.click(tuesday)
  expect(tuesday.getAttribute('aria-checked')).toBe('true')
  await user.click(tuesday)
  expect(tuesday.getAttribute('aria-checked')).toBe('false')
  await user.clear(name)
  await user.type(name, '新任务')
  const saveButton = screen.getByRole('button', { name: '保存修改' })
  const actions = requiredElement(saveButton.closest('aside'))
  const cancel = within(actions).getByRole('link', { name: '取消' })
  await user.click(saveButton)
  const warning = await within(actions).findByRole('alert')
  expect(warning.textContent).toContain('定时调度同步失败')
  expect(actions.children[1]).toBe(saveButton)
  expect(actions.children[2]).toBe(cancel)
  expect(actions.lastElementChild).toBe(warning)
  expect(screen.queryByRole('alert', { name: '操作失败通知' })).toBeNull()
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

it('uses the persisted IANA default for new tasks without dirtying the form', async () => {
  vi.mocked(GetPreferences).mockResolvedValueOnce({
    defaultTimezone: 'Europe/Berlin',
    schedulingEnabled: true,
  })
  vi.mocked(ListRooms).mockResolvedValue([])
  window.history.replaceState(null, '', '/#/tasks/new')
  const router = createAppRouter()
  render(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const select = await screen.findByRole<HTMLSelectElement>('combobox', { name: '时区' })
  await waitFor(() => expect(select.value).toBe('Europe/Berlin'))
  // The persisted zone is offered as one of the curated options.
  expect(select.selectedOptions[0]?.textContent).toContain('Europe/Berlin')
  expect(select.options.length).toBe(timezoneOptions.length)
})

it('picks the task timezone from a select and saves the chosen zone', async () => {
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 1,
      platformRoomId: '98765',
      title: '测试直播间',
      anchorName: '',
      remark: '',
      coverUrl: '',
      taskCount: 0,
    },
  ])
  vi.mocked(CreateTask).mockResolvedValue({
    id: 9,
    roomId: 1,
    name: '深夜问候',
    kind: TaskKind.Single,
    messages: ['晚上好'],
    intervalSeconds: null,
    maxCount: null,
    maxDurationSeconds: null,
    schedule: {
      kind: ScheduleKind.Daily,
      timezone: 'America/New_York',
      localDate: '',
      localTime: '21:00:00',
      weekdays: 0,
    },
    enabled: true,
    nextDueAt: null,
    createdAt: 0,
    updatedAt: 0,
  })
  window.history.replaceState(null, '', '/#/tasks/new')
  const user = userEvent.setup()
  const router = createAppRouter()
  render(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  await user.type(await screen.findByRole('textbox', { name: '任务名称' }), '深夜问候')
  await user.selectOptions(await screen.findByRole('combobox', { name: '目标直播间' }), '1')
  await user.type(screen.getByRole('textbox', { name: '发送队列' }), '晚上好')
  const select = screen.getByRole<HTMLSelectElement>('combobox', { name: '时区' })
  expect(select.value).toBe('Asia/Shanghai')
  await user.selectOptions(select, 'America/New_York')
  expect(select.value).toBe('America/New_York')

  await user.click(screen.getByRole('button', { name: '创建任务' }))
  await waitFor(() => expect(window.location.hash).toBe('#/tasks'))
  expect(vi.mocked(CreateTask)).toHaveBeenLastCalledWith(
    expect.objectContaining({
      schedule: expect.objectContaining({ timezone: 'America/New_York' }),
    }),
  )
})

it('asks before discarding dirty edits and keeps them when the user continues editing', async () => {
  vi.mocked(ListRooms).mockResolvedValue([
    {
      id: 1,
      platformRoomId: '98765',
      title: '测试直播间',
      anchorName: '',
      remark: '',
      coverUrl: '',
      taskCount: 0,
    },
  ])
  window.history.replaceState(null, '', '/#/tasks/new')
  const user = userEvent.setup()
  render(App, {
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })
  const name = await screen.findByRole<HTMLInputElement>('textbox', { name: '任务名称' })
  await user.type(name, '未保存的任务')
  await user.click(screen.getByRole('link', { name: '返回任务列表' }))

  const dialog = await screen.findByRole('dialog', { name: '放弃未保存的修改？' })
  expect(dialog.textContent).toContain('离开后，当前任务尚未保存的修改会丢失。')
  await user.click(within(dialog).getByRole('button', { name: '继续编辑' }))

  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: '放弃未保存的修改？' })).toBeNull(),
  )
  expect(window.location.hash).toBe('#/tasks/new')
  expect(name.value).toBe('未保存的任务')

  await user.click(screen.getByRole('link', { name: '返回任务列表' }))
  const confirmDialog = await screen.findByRole('dialog', { name: '放弃未保存的修改？' })
  await user.click(within(confirmDialog).getByRole('button', { name: '放弃更改' }))

  await waitFor(() => expect(window.location.hash).toBe('#/tasks'))
})
