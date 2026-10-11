import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createAppQueryClient } from '@/app/queryClient'
import { initTheme, useTheme } from '@/app/theme'
import SettingsPage from './SettingsPage.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  CheckAccount: vi.fn().mockResolvedValue({ state: 'unconfigured' }),
  SaveAccount: vi.fn(),
  ClearAccount: vi.fn(),
  TestSend: vi.fn(),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  ListRooms: vi.fn().mockResolvedValue([]),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice', () => ({
  GetCloseBehavior: vi.fn().mockResolvedValue('hide'),
  SetCloseBehavior: vi.fn().mockResolvedValue(undefined),
}))

it('switches and restores the theme with visible feedback and safe storage failure', async () => {
  const listeners: ((event: MediaQueryListEvent) => void)[] = []
  vi.stubGlobal('matchMedia', () => ({
    matches: false,
    addEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) => {
      listeners.push(listener)
    },
  }))
  useTheme().setPreference('dark')
  initTheme()

  const user = userEvent.setup()
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
  const restore = screen.getByRole<HTMLButtonElement>('button', { name: '恢复默认主题' })

  expect(restore.disabled).toBe(true)
  await user.click(screen.getByRole('radio', { name: '浅色' }))
  expect(screen.getByRole('radio', { name: '浅色' }).getAttribute('aria-checked')).toBe('true')
  expect(screen.getByRole('status').textContent).toContain('当前显示：浅色')
  expect(document.documentElement.dataset.theme).toBe('light')
  expect(localStorage.getItem('dan-tick-theme')).toBe('light')
  expect(restore.disabled).toBe(false)

  await user.click(screen.getByRole('radio', { name: '跟随系统' }))
  for (const listener of listeners) listener({ matches: true } as MediaQueryListEvent)
  await waitFor(() => {
    expect(screen.getByRole('status').textContent).toContain('当前显示：深色')
  })
  expect(document.documentElement.dataset.theme).toBe('dark')

  await user.click(restore)
  expect(restore.disabled).toBe(true)
  expect(localStorage.getItem('dan-tick-theme')).toBe('dark')

  const write = vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
    throw new Error('storage denied')
  })
  await user.click(screen.getByRole('radio', { name: '浅色' }))
  expect(screen.getByRole('alert').textContent).toContain('无法保存主题偏好')
  expect(screen.getByRole('status').textContent).toContain('当前显示：浅色')
  write.mockRestore()
  await user.click(restore)
  expect(restore.disabled).toBe(true)
})
