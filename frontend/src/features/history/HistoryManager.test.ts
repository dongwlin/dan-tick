import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { ListExecutionHistory } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/historyservice'
import type { Execution } from '@bindings/github.com/dongwlin/dan-tick/internal/history/models'
import { createAppRouter } from '@/app/router'
import { createAppQueryClient } from '@/app/queryClient'
import HistoryManager from './HistoryManager.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/historyservice', () => ({
  ListExecutionHistory: vi.fn(),
  GetExecutionHistory: vi.fn(),
}))

const rows: Execution[] = Array.from({ length: 9 }, (_, index) => ({
  id: 9 - index,
  task_id: 2,
  trigger_kind: 'manual',
  scheduled_at: null,
  status: 'completed',
  created_at: 1700000000000,
  queued_at: null,
  started_at: 1700000000001,
  finished_at: 1700000000002,
  sent_count: 1,
  failed_count: 0,
  error_code: null,
  task_name_snapshot: `历史任务 ${9 - index}`,
  platform_room_id_snapshot: '9988',
}))

it('uses window height for stable numbered pages and an end marker on the short last page', async () => {
  let height = 500
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(() => height)
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockImplementation(() => 60)
  vi.spyOn(HTMLElement.prototype, 'scrollTo').mockImplementation(() => {})
  vi.mocked(ListExecutionHistory).mockImplementation(
    (page, size) =>
      Promise.resolve({
        page,
        pageSize: size,
        total: rows.length,
        totalPages: Math.ceil(rows.length / size),
        items: rows.slice((page - 1) * size, page * size),
      }) as ReturnType<typeof ListExecutionHistory>,
  )
  const router = createAppRouter()
  const user = userEvent.setup()
  render(HistoryManager, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  await waitFor(() => expect(vi.mocked(ListExecutionHistory)).toHaveBeenCalledWith(1, 4))
  expect(await screen.findByText('历史任务 9')).toBeTruthy()
  const historyPanel = screen.getByRole('heading', { name: /执行历史/ }).parentElement
    ?.parentElement
  expect(historyPanel?.classList.contains('acrylic')).toBe(true)
  expect(historyPanel?.classList.contains('bg-surface-solid')).toBe(false)
  expect(historyPanel?.classList.contains('shadow-sm')).toBe(false)
  expect(screen.getByRole('columnheader', { name: '计划执行' })).toBeTruthy()
  expect(screen.getByRole('columnheader', { name: '实际开始' })).toBeTruthy()
  expect(
    screen.getByRole('columnheader', { name: '发送结果' }).classList.contains('text-right'),
  ).toBe(true)
  expect(
    screen
      .getAllByRole('cell')
      .find((cell) => cell.textContent?.includes('成功'))
      ?.classList.contains('text-right'),
  ).toBe(true)
  expect(screen.queryByText('历史任务 5')).toBeNull()
  expect(screen.getByRole('navigation', { name: '执行记录分页' })).toBeTruthy()
  await user.click(screen.getByRole('button', { name: '第 3 页' }))
  await waitFor(() => expect(vi.mocked(ListExecutionHistory)).toHaveBeenCalledWith(3, 4))
  const currentPage = screen.getByRole('button', { name: '第 3 页' })
  expect(currentPage.classList.contains('size-11')).toBe(true)
  expect(currentPage.classList.contains('bg-accent-solid')).toBe(true)
  expect(currentPage.getAttribute('aria-current')).toBe('page')
  expect(await screen.findByText('历史任务 1')).toBeTruthy()
  expect(screen.getByText('已到最后一页，共 9 条记录')).toBeTruthy()
  expect(screen.getByRole('button', { name: '下一页' }).hasAttribute('disabled')).toBe(true)

  // On resize all nine rows fit. The page clamps back to one, without a footer.
  height = 780
  window.dispatchEvent(new Event('resize'))
  await waitFor(() => expect(vi.mocked(ListExecutionHistory)).toHaveBeenCalledWith(1, 9))
  expect(await screen.findByText('历史任务 9')).toBeTruthy()
  expect(screen.queryByRole('navigation', { name: '执行记录分页' })).toBeNull()
  expect(screen.getByText('共 9 条记录，已全部显示')).toBeTruthy()
})

it('handles empty history and safe binding errors with retry', async () => {
  vi.mocked(ListExecutionHistory)
    .mockRejectedValueOnce(new Error('private database details'))
    .mockResolvedValue({ page: 1, pageSize: 6, total: 0, totalPages: 0, items: [] })
  const user = userEvent.setup()
  render(HistoryManager, {
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })
  expect((await screen.findByRole('alert')).textContent).toContain('操作失败，请重试或查看诊断日志')
  expect(screen.queryByText('private database details')).toBeNull()
  await user.click(screen.getByRole('button', { name: '重试读取' }))
  const emptyState = await screen.findByText('还没有执行记录')
  expect(emptyState.parentElement?.classList.contains('justify-center')).toBe(true)
  expect(emptyState.parentElement?.querySelector('svg')).not.toBeNull()
  expect(screen.getByLabelText('执行记录表格').classList.contains('flex')).toBe(true)
  expect(screen.queryByRole('navigation', { name: '执行记录分页' })).toBeNull()
})

it('marks a test send in the history list without treating it as a task run', async () => {
  const testSend: Execution = {
    id: 21,
    task_id: null,
    trigger_kind: 'test',
    scheduled_at: null,
    status: 'completed',
    created_at: 1700000000000,
    queued_at: 1700000000001,
    started_at: 1700000000002,
    finished_at: 1700000000003,
    sent_count: 1,
    failed_count: 0,
    error_code: null,
    task_name_snapshot: '测试发送',
    platform_room_id_snapshot: '12345',
  }
  vi.mocked(ListExecutionHistory).mockResolvedValue({
    page: 1,
    pageSize: 6,
    total: 1,
    totalPages: 1,
    items: [testSend],
  })
  render(HistoryManager, {
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })
  expect(await screen.findByText('测试发送')).toBeTruthy()
  expect(screen.getByText('直播间 12345')).toBeTruthy()
  expect(screen.getByRole('link', { name: '查看 测试发送 的执行详情' }).getAttribute('href')).toBe(
    '#/history/21',
  )
})
