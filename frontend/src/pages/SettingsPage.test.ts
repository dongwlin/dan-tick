import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import { CheckAccount } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { GetBuildInfo } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice'
import { createAppQueryClient } from '@/app/queryClient'
import { initTheme, useTheme } from '@/app/theme'
import { operationKeys } from '@/features/operations/queries'
import SettingsPage from './SettingsPage.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  CheckAccount: vi.fn().mockResolvedValue({ state: 'unconfigured' }),
  BeginQRLogin: vi.fn(),
  PollQRLogin: vi.fn(),
  ConfirmQRAccount: vi.fn(),
  CancelQRLogin: vi.fn().mockResolvedValue(undefined),
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
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/autostartservice', () => ({
  GetAutostartStatus: vi
    .fn()
    .mockResolvedValue({ enabled: false, supported: true, needsRepair: false }),
  SetAutostartEnabled: vi.fn(),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice', () => ({
  GetBuildInfo: vi.fn().mockResolvedValue({ version: 'dev', buildTime: '', commit: '' }),
  GetPreferences: vi
    .fn()
    .mockResolvedValue({ defaultTimezone: 'Asia/Shanghai', schedulingEnabled: true }),
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

it('renders settings cards without shadows', () => {
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  for (const name of [
    '账号资料',
    'B站账号凭证',
    '测试发送',
    '窗口与后台运行',
    '开机自启动',
    '调度设置',
    '界面主题',
    '数据与日志',
    '关于',
  ]) {
    expect(screen.getByRole('region', { name }).classList.contains('shadow-sm')).toBe(false)
  }
  expect(screen.getByRole('heading', { name: '设置' }).closest('header')).not.toBeNull()
})

it('places autostart after window behavior and before scheduling', () => {
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  const autostart = screen.getByRole('region', { name: '开机自启动' })
  expect(screen.getByRole('region', { name: '窗口与后台运行' }).nextElementSibling).toBe(autostart)
  expect(autostart.nextElementSibling).toBe(screen.getByRole('region', { name: '调度设置' }))
})

it('places the about section after diagnostics with the existing brand mark', () => {
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  const diagnostics = screen.getByRole('region', { name: '数据与日志' })
  const about = screen.getByRole('region', { name: '关于' })
  expect(diagnostics.nextElementSibling).toBe(about)
  expect(about.querySelector('img')?.getAttribute('alt')).toBe('dan-tick')
  expect(about.textContent).toContain('版本号')
  expect(about.textContent).toContain('构建时间')
  expect(about.textContent).toContain('Git Commit')
})

it('loads compiled metadata through the Go binding and displays it in the about section', async () => {
  vi.mocked(GetBuildInfo).mockResolvedValueOnce({
    version: 'v1.2.3-a1b2c3d4',
    buildTime: '2026-10-13T08:12:00Z',
    commit: 'a1b2c3d4e5f67890',
  })
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  const about = within(screen.getByRole('region', { name: '关于' }))
  expect(await about.findByText('v1.2.3-a1b2c3d4')).toBeTruthy()
  expect(about.getByText('Git Commit').nextElementSibling?.textContent).toBe('a1b2c3d4')
  expect(about.getByText('构建时间').nextElementSibling?.textContent).toContain('2026')
  expect(about.getByText('构建时间').nextElementSibling?.textContent).not.toBe('未提供')
})

it('keeps the about placeholders and gives safe feedback when the binding fails', async () => {
  vi.mocked(GetBuildInfo).mockRejectedValueOnce(new Error('private desktop detail'))
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  const about = within(screen.getByRole('region', { name: '关于' }))
  expect((await about.findByRole('alert')).textContent).toContain('无法读取构建信息')
  expect(about.getByText('版本号').nextElementSibling?.textContent).toBe('dev')
  expect(about.getByText('构建时间').nextElementSibling?.textContent).toBe('未提供')
  expect(about.getByText('Git Commit').nextElementSibling?.textContent).toBe('未提供')
  expect(screen.queryByText('private desktop detail')).toBeNull()
})

it('shows the verified nickname, avatar and UID above credentials, then clears them on expiry', async () => {
  const client = createAppQueryClient()
  vi.mocked(CheckAccount).mockResolvedValueOnce({
    state: State.LoggedIn,
    uid: '123456789',
    name: '测试用户',
    avatarUrl: 'https://i0.hdslb.com/bfs/face/avatar.jpg',
  })
  render(SettingsPage, { global: { plugins: [[VueQueryPlugin, { queryClient: client }]] } })

  const profile = screen.getByRole('region', { name: '账号资料' })
  expect(profile.nextElementSibling).toBe(screen.getByRole('region', { name: 'B站账号凭证' }))
  expect(await screen.findByText('测试用户')).toBeTruthy()
  expect(screen.getByText('UID：123456789')).toBeTruthy()
  const avatar = profile.querySelector('img')
  expect(avatar?.getAttribute('src')).toBe('https://i0.hdslb.com/bfs/face/avatar.jpg')
  // Reka keeps the fallback mounted until the image reports it loaded, so the
  // decorative icon remains available when the avatar request fails.
  expect(profile.querySelector('svg')).toBeTruthy()

  client.setQueryData(operationKeys.account, { state: State.Expired })
  await waitFor(() => expect(screen.getByText('登录已过期')).toBeTruthy())
  expect(screen.getByText('UID：—')).toBeTruthy()
  expect(screen.queryByText('测试用户')).toBeNull()
})

it('keeps settings cards inset within a full-width scroll area', () => {
  render(SettingsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })

  const scrollArea = screen.getByLabelText('设置内容')
  expect(scrollArea.classList.contains('overflow-y-auto')).toBe(true)
  expect(scrollArea.tabIndex).toBe(0)
  expect(scrollArea.firstElementChild?.classList.contains('px-7')).toBe(true)
  expect(scrollArea.firstElementChild?.classList.contains('max-w-[1520px]')).toBe(true)
  expect(screen.getByRole('heading', { name: '设置' }).parentElement).not.toBe(scrollArea)
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
