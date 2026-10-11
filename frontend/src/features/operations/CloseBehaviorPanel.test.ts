import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import {
  GetCloseBehavior,
  SetCloseBehavior,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice'
import { createAppQueryClient } from '@/app/queryClient'
import CloseBehaviorPanel from './CloseBehaviorPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice', () => ({
  GetCloseBehavior: vi.fn(),
  SetCloseBehavior: vi.fn(),
}))

it('reads native close behavior, updates it and reports persistence failures', async () => {
  const user = userEvent.setup()
  vi.mocked(GetCloseBehavior).mockResolvedValue('hide')
  vi.mocked(SetCloseBehavior)
    .mockRejectedValueOnce(new Error('private failure'))
    .mockResolvedValue(undefined)
  render(CloseBehaviorPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const hide = await screen.findByRole('radio', { name: /关闭时隐藏窗口/ })
  const quit = screen.getByRole('radio', { name: /关闭时退出应用/ })
  await waitFor(() => expect(hide.getAttribute('aria-checked')).toBe('true'))

  await user.click(quit)
  expect((await screen.findByRole('alert')).textContent).toContain('保存窗口偏好失败，设置未更改。')
  expect(hide.getAttribute('aria-checked')).toBe('true')
  expect(quit.getAttribute('aria-checked')).toBe('false')

  vi.mocked(GetCloseBehavior).mockResolvedValue('quit')
  await user.click(quit)
  await waitFor(() => expect(quit.getAttribute('aria-checked')).toBe('true'))
  expect(SetCloseBehavior).toHaveBeenCalledWith('quit')
})

it('offers retry if the preference cannot be read', async () => {
  vi.mocked(GetCloseBehavior)
    .mockRejectedValueOnce(new Error('private failure'))
    .mockResolvedValue('hide')
  render(CloseBehaviorPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  expect((await screen.findByRole('alert')).textContent).toContain('读取窗口偏好失败')
  await userEvent.setup().click(screen.getByRole('button', { name: '重试' }))
  await waitFor(() =>
    expect(screen.getByRole('radio', { name: /关闭时隐藏窗口/ }).getAttribute('aria-checked')).toBe(
      'true',
    ),
  )
})
