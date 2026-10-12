import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import UiAsyncSection from './UiAsyncSection.vue'

it('announces loading and renders the default slot once data arrives', async () => {
  const view = render(UiAsyncSection, {
    props: { pending: true, loadingText: '正在加载任务…' },
    slots: { default: '任务内容' },
  })

  expect(screen.getByRole('status').textContent).toBe('正在加载任务…')
  expect(screen.queryByText('任务内容')).toBeNull()

  await view.rerender({ pending: false })
  expect(screen.queryByRole('status')).toBeNull()
  expect(screen.getByText('任务内容')).toBeTruthy()
})

it('shows a safe failure message and emits retry from the shared action', async () => {
  const retry = vi.fn()
  render(UiAsyncSection, {
    props: { pending: false, error: new Error('private SQL details') },
    slots: { default: '任务内容' },
    attrs: { onRetry: retry },
  })

  const alert = screen.getByRole('alert')
  expect(alert.textContent).toContain('操作失败，请重试或查看诊断日志')
  expect(alert.textContent).not.toContain('private SQL details')
  await userEvent.setup().click(screen.getByRole('button', { name: '重试读取' }))
  expect(retry).toHaveBeenCalledOnce()
  expect(screen.queryByText('任务内容')).toBeNull()
})
