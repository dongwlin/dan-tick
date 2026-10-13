import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import { CheckAccount } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import App from './App.vue'
import { createAppRouter } from './router'
import { createAppQueryClient } from './queryClient'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListRoomPage: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
  ListTaskPage: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
  ListRooms: vi.fn().mockResolvedValue([]),
  GetTask: vi.fn().mockRejectedValue(new Error('操作失败，请重试')),
}))

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  CheckAccount: vi.fn().mockResolvedValue({ state: 'unconfigured' }),
  LatestExecutions: vi.fn().mockResolvedValue([]),
  ClearAccount: vi.fn(),
  BeginQRLogin: vi.fn(),
  PollQRLogin: vi.fn(),
  ConfirmQRAccount: vi.fn(),
  CancelQRLogin: vi.fn().mockResolvedValue(undefined),
  TestSend: vi.fn(),
  StartManual: vi.fn(),
  StopExecution: vi.fn(),
}))

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/historyservice', () => ({
  ListExecutionHistory: vi.fn().mockResolvedValue({
    page: 1,
    pageSize: 6,
    total: 0,
    totalPages: 0,
    items: [],
  }),
  GetExecutionHistory: vi.fn(),
}))

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice', () => ({
  GetPreferences: vi
    .fn()
    .mockResolvedValue({ defaultTimezone: 'Asia/Shanghai', schedulingEnabled: true }),
  GetOverview: vi.fn().mockResolvedValue({
    totalTasks: 0,
    enabledTasks: 0,
    activeExecutions: 0,
    roomCount: 0,
    upcoming: [],
    recent: [],
    latestFailure: null,
  }),
  SetDefaultTimezone: vi.fn(),
  SetSchedulingEnabled: vi.fn(),
}))

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/diagnosticsservice', () => ({
  GetDiagnosticsStatus: vi.fn().mockResolvedValue({
    directory: '/tmp/logs',
    dataDirectory: '/tmp/data',
    degraded: false,
    warning: '',
  }),
  OpenLogDirectory: vi.fn(),
  OpenDataDirectory: vi.fn(),
  ExportDiagnostics: vi.fn(),
}))

function requiredElement<T extends Element>(element: T | null): T {
  if (!element) throw new Error('Expected an element to be present')
  return element
}

function renderAtHash(path: string) {
  window.history.replaceState(null, '', `/#${path}`)
  const router = createAppRouter()
  render(App, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  return userEvent.setup()
}

describe('application navigation', () => {
  it('displays a static account avatar without hover effects or navigation', async () => {
    vi.mocked(CheckAccount).mockResolvedValueOnce({
      state: State.LoggedIn,
      uid: '123456789',
      name: '测试用户',
      avatarUrl: 'https://i0.hdslb.com/bfs/face/avatar.jpg',
    })
    const user = renderAtHash('/')
    const avatar = screen.getByRole('img', { name: 'B 站账号头像' })
    await waitFor(() =>
      expect(avatar.querySelector('img')?.getAttribute('src')).toBe(
        'https://i0.hdslb.com/bfs/face/avatar.jpg',
      ),
    )
    expect(avatar.textContent?.trim()).toBe('')
    expect(avatar.getAttribute('title')).toBeNull()
    expect(avatar.className).not.toContain('hover:')
    expect(screen.queryByRole('link', { name: '打开 B 站账号设置' })).toBeNull()
    await user.hover(avatar)
    await user.click(avatar)
    expect(window.location.hash).toBe('#/')
    await user.click(screen.getByRole('link', { name: '设置' }))
    expect(await screen.findByText('测试用户')).toBeTruthy()
    expect(screen.getByText('UID：123456789')).toBeTruthy()
  })

  it('describes the icon-only navigation links with an accessible tooltip', async () => {
    const user = renderAtHash('/')
    const overview = await screen.findByRole('link', { name: '总览' })

    overview.focus()
    await waitFor(() => expect(overview.getAttribute('aria-describedby')).toBeTruthy())
    const tooltipId = overview.getAttribute('aria-describedby')
    if (!tooltipId) throw new Error('Expected a tooltip relationship')
    expect(document.getElementById(tooltipId)?.textContent).toContain('总览')

    await user.keyboard('{Escape}')
    await waitFor(() => expect(overview.hasAttribute('aria-describedby')).toBe(false))
  })

  it('navigates from a deep link using the real Hash Router', async () => {
    const user = renderAtHash('/rooms')

    expect(await screen.findByRole('heading', { name: '直播间' })).toBeTruthy()
    expect(document.querySelector('.app-content')?.classList.contains('overflow-y-auto')).toBe(true)
    expect(screen.queryByText('dan-tick · 本地安全调度与执行')).toBeNull()
    await user.click(screen.getByRole('link', { name: '设置' }))

    expect(await screen.findByRole('heading', { name: '设置' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: '设置' }).closest('header')).not.toBeNull()
    expect(window.location.hash).toBe('#/settings')
    const appContent = document.querySelector('.app-content')
    expect(appContent?.classList.contains('overflow-hidden')).toBe(true)
    expect(appContent?.classList.contains('px-7')).toBe(false)
  })

  it('keeps one toast viewport for the settings panels that can raise feedback', async () => {
    renderAtHash('/settings')

    expect(await screen.findByRole('heading', { name: '设置' })).toBeTruthy()
    expect(screen.getAllByRole('region', { name: '通知 (F8)' })).toHaveLength(1)
  })

  it('offers a working return link on an unknown deep link', async () => {
    const user = renderAtHash('/does-not-exist')

    expect(await screen.findByRole('heading', { name: '页面不存在' })).toBeTruthy()
    await user.click(screen.getByRole('link', { name: '返回总览' }))

    expect(await screen.findByRole('heading', { name: '总览' })).toBeTruthy()
    expect(window.location.hash).toBe('#/')
  })

  it('highlights parent navigation for editing and detail routes', async () => {
    const user = renderAtHash('/tasks/123/edit')
    expect(await screen.findByRole('heading', { name: '编辑任务' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: '编辑任务' }).closest('header')).not.toBeNull()
    expect(screen.getByRole('link', { name: '弹幕任务' }).getAttribute('aria-current')).toBe('page')

    await user.click(screen.getByRole('link', { name: '执行记录' }))
    expect(await screen.findByRole('heading', { name: '执行记录' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '执行记录' }).getAttribute('aria-current')).toBe('page')
    await user.click(screen.getByRole('link', { name: '设置' }))
    expect(await screen.findByRole('heading', { name: '设置' })).toBeTruthy()
  })
})
