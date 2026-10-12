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
      { path: '/history/:id', component: { template: '<p>详情</p>' } },
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
  expect(screen.getByText(/已暂停，以下计划目前不会触发/)).toBeTruthy()
  expect(screen.getByText('失败的任务')).toBeTruthy()
  expect(screen.getByRole('link', { name: '新任务' }).getAttribute('href')).toBe('/tasks/11/edit')
  expect(screen.getByRole('link', { name: '查看执行详情' }).getAttribute('href')).toBe(
    '/history/17',
  )
  expect(screen.getByText('Europe/Berlin')).toBeTruthy()
  expect(screen.getByText('未配置')).toBeTruthy()
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
  expect(document.body.textContent).not.toContain('private SQL text')
  screen.getByRole('button', { name: '重试读取' }).click()
  expect(await screen.findByText('暂无即将执行的任务')).toBeTruthy()
  expect(screen.getByText('还没有执行记录。运行任务后可在此查看结果。')).toBeTruthy()
})
