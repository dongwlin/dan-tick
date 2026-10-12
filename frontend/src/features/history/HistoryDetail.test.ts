import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen } from '@testing-library/vue'
import { expect, it, vi } from 'vitest'
import { GetExecutionHistory } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/historyservice'
import { createAppRouter } from '@/app/router'
import { createAppQueryClient } from '@/app/queryClient'
import HistoryDetail from './HistoryDetail.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/historyservice', () => ({
  ListExecutionHistory: vi.fn(),
  GetExecutionHistory: vi.fn(),
}))

function mount(id: number) {
  render(HistoryDetail, {
    props: { executionId: id },
    global: {
      plugins: [createAppRouter(), [VueQueryPlugin, { queryClient: createAppQueryClient() }]],
    },
  })
}

it('shows persisted task snapshots, lifecycle times, ordered attempts and safe error descriptions', async () => {
  vi.mocked(GetExecutionHistory).mockResolvedValue({
    execution: {
      id: 12,
      task_id: 3,
      trigger_kind: 'scheduled',
      scheduled_at: 1700000000000,
      status: 'failed',
      created_at: 1700000000000,
      queued_at: 1700000000010,
      started_at: 1700000000020,
      finished_at: 1700000000030,
      sent_count: 1,
      failed_count: 1,
      error_code: 'authentication',
      task_name_snapshot: '已删除任务的快照',
      platform_room_id_snapshot: '9988',
    },
    attempts: [
      {
        id: 2,
        execution_id: 12,
        sequence: 1,
        message_no: 1,
        content_snapshot: '消息快照甲',
        attempted_at: 1700000000021,
        result: 'accepted',
        error_code: null,
      },
      {
        id: 3,
        execution_id: 12,
        sequence: 2,
        message_no: 2,
        content_snapshot: '消息快照乙',
        attempted_at: 1700000000022,
        result: 'unknown',
        error_code: 'recovery_unknown',
      },
    ],
  })
  mount(12)
  expect(await screen.findByRole('heading', { name: '已删除任务的快照' })).toBeTruthy()
  const summary = screen.getByRole('heading', { name: '已删除任务的快照' }).closest('.acrylic')
  expect(summary?.classList.contains('shadow-sm')).toBe(false)
  expect(screen.getByLabelText('发送尝试详情').parentElement?.classList.contains('shadow-sm')).toBe(
    false,
  )
  expect(screen.getByText('直播间 9988 · 计划执行')).toBeTruthy()
  expect(screen.getByText('登录已失效')).toBeTruthy()
  expect(screen.getByText('消息快照甲')).toBeTruthy()
  expect(screen.getByText('消息快照乙')).toBeTruthy()
  expect(screen.getByText('恢复时无法确认发送结果')).toBeTruthy()
  expect(screen.getByText('第 2 次尝试 · 消息 2')).toBeTruthy()
  expect(vi.mocked(GetExecutionHistory)).toHaveBeenCalledWith(12)
})

it('rejects invalid route ids without issuing a binding call', () => {
  mount(0)
  expect(screen.getByRole('alert').textContent).toContain('无效的执行记录编号')
  expect(screen.getByRole('alert').classList.contains('shadow-sm')).toBe(false)
  expect(vi.mocked(GetExecutionHistory)).not.toHaveBeenCalled()
})

it('shows a safe not-found message for deleted history without exposing database details', async () => {
  vi.mocked(GetExecutionHistory).mockRejectedValue(new Error('指定执行记录不存在'))
  mount(99999)
  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    expect.stringContaining('指定执行记录不存在'),
  )
  expect(vi.mocked(GetExecutionHistory)).toHaveBeenCalledWith(99999)
})
