import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import {
  StartManual,
  StopExecution,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { createAppQueryClient } from '@/app/queryClient'
import TaskRunActions from './TaskRunActions.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  StartManual: vi.fn(),
  StopExecution: vi.fn(),
}))

it('starts a manual run and stops an active execution without coupling to page lifetime', async () => {
  const user = userEvent.setup()
  vi.mocked(StartManual).mockResolvedValue({ executionId: 77 })
  vi.mocked(StopExecution).mockResolvedValue(undefined)
  const client = createAppQueryClient()
  const view = render(TaskRunActions, {
    props: { taskId: 3 },
    global: { plugins: [[VueQueryPlugin, { queryClient: client }]] },
  })
  await user.click(screen.getByRole('button', { name: '立即执行任务 3' }))
  await waitFor(() => expect(vi.mocked(StartManual)).toHaveBeenCalledWith(3))
  expect(await screen.findByText(/已创建执行 #77/)).toBeTruthy()

  await view.rerender({
    taskId: 3,
    execution: { id: 77, taskId: 3, status: 'running', sentCount: 1, failedCount: 0 },
  })
  expect(screen.getByText(/正在执行 · 成功 1/)).toBeTruthy()
  await user.click(screen.getByRole('button', { name: '停止任务 3 当前执行' }))
  await waitFor(() => expect(vi.mocked(StopExecution)).toHaveBeenCalledWith(77))
  expect(await screen.findByText(/已请求停止/)).toBeTruthy()
  view.unmount()
  expect(vi.mocked(StopExecution)).toHaveBeenCalledTimes(1)
})
