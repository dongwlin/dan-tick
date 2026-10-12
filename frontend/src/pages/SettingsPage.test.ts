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
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice', () => ({
  GetPreferences: vi
    .fn()
    .mockResolvedValue({ defaultTimezone: 'Asia/Shanghai', schedulingEnabled: true }),
  SetDefaultTimezone: vi.fn(),
  SetSchedulingEnabled: vi.fn(),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/diagnosticsservice', () => ({
  GetDiagnosticsStatus: vi
    .fn()
    .mockResolvedValue({ directory: '/tmp/logs', degraded: false, warning: '' }),
  OpenLogDirectory: vi.fn(),
  OpenDataDirectory: vi.fn(),
  ExportDiagnostics: vi.fn(),
}))

it('renders settings cards without shadows', () => {
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  for (const name of [
    'B 站账号',
    '测试发送',
    '窗口与后台运行',
    '调度设置',
    '界面主题',
    '数据与日志',
  ]) {
    expect(screen.getByRole('region', { name }).classList.contains('shadow-sm')).toBe(false)
  }
})

it('switches the theme with persistent preferences and warns when storage fails', async () => {
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
  const section = screen.getByRole('region', { name: '界面主题' })
  expect(section.tagName).toBe('SECTION')
  expect(section.querySelector('h2')?.textContent).toBe('界面主题')
  expect(section.querySelector('legend')).toBeNull()
  expect(screen.queryByText('仅保存显示偏好，不保存业务数据')).toBeNull()
  expect(screen.queryByText(/^当前显示：/)).toBeNull()
  expect(screen.queryByRole('button', { name: '恢复默认主题' })).toBeNull()
  await user.click(screen.getByRole('radio', { name: '浅色' }))
  expect(screen.getByRole('radio', { name: '浅色' }).getAttribute('aria-checked')).toBe('true')
  expect(document.documentElement.dataset.theme).toBe('light')
  expect(localStorage.getItem('dan-tick-theme')).toBe('light')

  await user.click(screen.getByRole('radio', { name: '跟随系统' }))
  for (const listener of listeners) listener({ matches: true } as MediaQueryListEvent)
  await waitFor(() => {
    expect(document.documentElement.dataset.theme).toBe('dark')
  })
  expect(localStorage.getItem('dan-tick-theme')).toBe('system')

  await user.click(screen.getByRole('radio', { name: '深色' }))
  expect(localStorage.getItem('dan-tick-theme')).toBe('dark')

  const write = vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
    throw new Error('storage denied')
  })
  await user.click(screen.getByRole('radio', { name: '浅色' }))
  expect(screen.getByRole('alert').textContent).toContain('无法保存主题偏好')
  expect(document.documentElement.dataset.theme).toBe('light')
  write.mockRestore()
  await user.click(screen.getByRole('radio', { name: '深色' }))
  expect(localStorage.getItem('dan-tick-theme')).toBe('dark')
})
