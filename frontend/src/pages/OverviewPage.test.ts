import { render, screen } from '@testing-library/vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, expect, it, vi } from 'vitest'
import { createAppQueryClient } from '@/app/queryClient'
import {
  GetOverview,
  GetPreferences,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice'
import { CheckAccount } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { State as AccountState } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import type { Execution } from '@bindings/github.com/dongwlin/dan-tick/internal/history/models'
import OverviewPage from './OverviewPage.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice', () => ({
  GetOverview: vi.fn(),
  GetPreferences: vi.fn(),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  CheckAccount: vi.fn(),
}))

async function show() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: OverviewPage },
      { path: '/tasks/new', component: { template: '<p>新建</p>' } },
      { path: '/tasks/:id/edit', component: { template: '<p>编辑</p>' } },
      { path: '/settings', component: { template: '<p>设置</p>' } },
      { path: '/tasks', component: { template: '<p>任务</p>' } },
      { path: '/history', component: { template: '<p>历史</p>' } },
      { path: '/history/:id', name: 'history-detail', component: { template: '<p>详情</p>' } },
      { path: '/rooms', component: { template: '<p>直播间</p>' } },
    ],
  })
  await router.push('/')
  await router.isReady()
  render(OverviewPage, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
}

beforeEach(() => {
  vi.mocked(GetOverview).mockReset()
  vi.mocked(GetPreferences).mockReset().mockResolvedValue({
    defaultTimezone: 'Europe/Berlin',
    schedulingEnabled: false,
  })
  vi.mocked(CheckAccount).mockReset().mockResolvedValue({ state: AccountState.Unconfigured })
})

it('shows live task summaries, a paused schedule notice, and navigable failure details', async () => {
  vi.mocked(GetOverview).mockResolvedValue({
    totalTasks: 7,
    enabledTasks: 2,
    activeExecutions: 1,
    roomCount: 3,
    upcoming: [
      {
        id: 11,
        name: '新任务',
        room: '123456',
        kind: 'single',
        nextDueAt: 1792000000000,
        timezone: 'Europe/Berlin',
      },
    ],
    latestFailure: { id: 17, taskName: '失败的任务', status: 'failed', time: 1792000000000 },
    recent: [],
  })
  await show()
  expect(await screen.findByText('新任务')).toBeTruthy()
  const summary = screen.getByLabelText('系统状态')
  expect(summary.classList.contains('acrylic')).toBe(true)
  expect(summary.classList.contains('bg-surface-solid')).toBe(false)
  expect(summary.querySelectorAll('a')).toHaveLength(0)
  expect(summary.textContent).toContain('自动调度')
  expect(summary.textContent).toContain('B 站账号')
  expect(summary.textContent).toContain('已启用任务')
  expect(summary.textContent).toContain('正在执行 / 排队')
  expect(summary.children).toHaveLength(4)
  expect(summary.children[0]?.classList.contains('border-r')).toBe(true)
  expect(summary.children[3]?.classList.contains('border-r')).toBe(false)
  for (const title of ['即将执行', '最近异常', '最近执行']) {
    const heading = screen.getByRole('heading', { name: title })
    expect(heading.parentElement?.classList.contains('border-b')).toBe(false)
    expect(screen.getByRole('region', { name: title }).classList.contains('shadow-sm')).toBe(false)
  }
  expect(screen.getByText(/已暂停，以下计划目前不会触发/)).toBeTruthy()
  expect(screen.getByText('失败的任务')).toBeTruthy()
  expect(screen.getByRole('link', { name: '新任务' }).getAttribute('href')).toBe('/tasks/11/edit')
  expect(screen.getByRole('link', { name: '查看执行详情' }).getAttribute('href')).toBe(
    '/history/17',
  )
  expect(screen.getByText('Europe/Berlin')).toBeTruthy()
  expect(screen.getByText('未配置')).toBeTruthy()
  const roomCountLink = screen.getByRole('link', { name: '3 个' })
  expect(roomCountLink.getAttribute('href')).toBe('/rooms')
  const footerDivider = roomCountLink.parentElement
  expect(footerDivider?.classList.contains('border-t')).toBe(true)
  expect(footerDivider?.parentElement?.classList.contains('px-5')).toBe(true)
  expect(footerDivider?.parentElement?.classList.contains('border-t')).toBe(false)
  expect(roomCountLink.classList.contains('text-text-secondary')).toBe(true)
  expect(roomCountLink.classList.contains('text-accent-text')).toBe(false)
  expect(roomCountLink.classList.contains('hover:text-accent-text')).toBe(true)
  const recentPanel = screen.getByRole('region', { name: '最近执行' })
  expect(recentPanel.classList.contains('acrylic')).toBe(true)
  expect(recentPanel.querySelector('thead')?.classList.contains('bg-surface-solid')).toBe(false)
  expect(recentPanel.querySelector('thead')).not.toBeNull()
  expect(recentPanel.querySelector('tbody')?.classList.contains('overflow-y-auto')).toBe(true)
  expect(recentPanel.querySelector('tbody')?.getAttribute('tabindex')).toBe('0')
  expect(recentPanel.querySelector('tbody')?.classList.contains('flex')).toBe(true)
  expect(
    [...recentPanel.querySelectorAll('th')].map((heading) => heading.textContent?.trim()),
  ).toEqual(['任务 / 直播间', '执行时间', '发送结果', '状态', '操作'])
})

it('renders empty state and retries failed overview reads', async () => {
  vi.mocked(GetOverview)
    .mockRejectedValueOnce(new Error('private SQL text'))
    .mockResolvedValueOnce({
      totalTasks: 0,
      enabledTasks: 0,
      activeExecutions: 0,
      roomCount: 0,
      upcoming: [],
      recent: [],
      latestFailure: null,
    })
  await show()
  expect(await screen.findByRole('alert')).toBeTruthy()
  expect(screen.getByRole('alert').classList.contains('shadow-sm')).toBe(false)
  expect(document.body.textContent).not.toContain('private SQL text')
  screen.getByRole('button', { name: '重试读取' }).click()
  expect(await screen.findByText('暂无即将执行的任务')).toBeTruthy()
  expect(screen.queryByRole('link', { name: '创建任务' })).toBeNull()
  expect(screen.getByRole('link', { name: '新建任务' }).getAttribute('href')).toBe('/tasks/new')
  const emptyAlert = screen.getByText('最近没有失败、中断或错过的执行').parentElement
  expect(emptyAlert?.classList.contains('items-center')).toBe(true)
  expect(emptyAlert?.querySelector('svg')).not.toBeNull()
  expect(document.body.textContent).not.toContain('最近没有失败、中断或错过的执行。')
  const emptyRecent = screen.getByText('还没有执行记录')
  expect(emptyRecent.closest('td')?.classList.contains('items-center')).toBe(true)
  expect(emptyRecent.closest('td')?.querySelector('svg')).not.toBeNull()
  expect(document.body.textContent).not.toContain('运行任务后可在此查看结果')
  const recentPanel = screen.getByRole('region', { name: '最近执行' })
  expect(recentPanel.querySelector('thead')).not.toBeNull()
  expect(recentPanel.querySelector('tbody')?.classList.contains('overflow-y-auto')).toBe(true)
  expect(recentPanel.querySelector('tbody')?.textContent).toContain('还没有执行记录')
})

it('uses the actual execution time in the compact overview table', async () => {
  const recent: Execution = {
    id: 19,
    task_id: 4,
    trigger_kind: 'manual',
    scheduled_at: null,
    status: 'completed',
    created_at: 1700000000000,
    queued_at: null,
    started_at: 1700000001000,
    finished_at: 1700000002000,
    sent_count: 2,
    failed_count: 0,
    error_code: null,
    task_name_snapshot: '最近任务',
    platform_room_id_snapshot: '123456',
  }
  vi.mocked(GetOverview).mockResolvedValue({
    totalTasks: 1,
    enabledTasks: 1,
    activeExecutions: 0,
    roomCount: 1,
    upcoming: [],
    latestFailure: null,
    recent: [recent],
  })
  await show()
  expect(await screen.findByText('最近任务')).toBeTruthy()
  const recentPanel = screen.getByRole('region', { name: '最近执行' })
  expect(recentPanel.querySelector('tbody')?.textContent).toContain(
    new Date(1700000001000).toLocaleString('zh-CN', { hour12: false }),
  )
  expect(screen.getByRole('link', { name: '查看 最近任务 的执行详情' }).getAttribute('href')).toBe(
    '/history/19',
  )
})
