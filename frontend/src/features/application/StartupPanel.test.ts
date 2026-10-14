import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { CancellablePromise } from '@wailsio/runtime'
import { beforeEach, expect, it, vi } from 'vitest'
import {
  GetAutostartStatus,
  SetAutostartEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/autostartservice'
import {
  GetStartupVisibility,
  SetStartupVisibility,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice'
import { createAppQueryClient } from '@/app/queryClient'
import { mountToastHost } from '@/test/toastHost'
import StartupPanel from './StartupPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/autostartservice', () => ({
  GetAutostartStatus: vi.fn(),
  SetAutostartEnabled: vi.fn(),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice', () => ({
  GetStartupVisibility: vi.fn(),
  SetStartupVisibility: vi.fn(),
}))

const defaultStatus = { enabled: false, supported: true, needsRepair: false }

function show() {
  mountToastHost()
  return render(StartupPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
}

beforeEach(() => {
  vi.mocked(GetAutostartStatus).mockReset().mockResolvedValue(defaultStatus)
  vi.mocked(SetAutostartEnabled)
    .mockReset()
    .mockImplementation(
      (enabled) => new CancellablePromise((resolve) => resolve({ ...defaultStatus, enabled })),
    )
  vi.mocked(GetStartupVisibility).mockReset().mockResolvedValue('show')
  vi.mocked(SetStartupVisibility)
    .mockReset()
    .mockImplementation(() => new CancellablePromise((resolve) => resolve(undefined)))
})

it('loads an existing enabled registration from the backend', async () => {
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, enabled: true })
  show()

  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(screen.getByText('登录桌面会话后自动启动 dan-tick')).toBeTruthy()
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('enables and disables autostart using the confirmed system state', async () => {
  const user = userEvent.setup()
  show()

  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  await user.click(toggle)
  expect(SetAutostartEnabled).toHaveBeenCalledWith(true)
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '已开启开机自启动',
  )

  toggle.focus()
  await user.keyboard(' ')
  expect(SetAutostartEnabled).toHaveBeenLastCalledWith(false)
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '已关闭开机自启动',
  )
})

it('rereads the system after a failed confirmation and shows a change that already took effect', async () => {
  const user = userEvent.setup()
  let resolveRead: (status: typeof defaultStatus) => void = () => {}
  vi.mocked(GetAutostartStatus)
    .mockResolvedValueOnce(defaultStatus)
    .mockImplementationOnce(
      () =>
        new CancellablePromise((resolve) => {
          resolveRead = resolve
        }),
    )
  vi.mocked(SetAutostartEnabled).mockRejectedValueOnce(new Error('private registry path'))
  show()

  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  await user.click(toggle)
  await waitFor(() => expect(GetAutostartStatus).toHaveBeenCalledTimes(2))
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  await user.click(toggle)
  expect(SetAutostartEnabled).toHaveBeenCalledTimes(1)

  resolveRead({ ...defaultStatus, enabled: true })
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '更改开机自启动未成功确认',
  )
  expect(toggle.hasAttribute('disabled')).toBe(false)
  expect(document.body.textContent).not.toContain('private registry path')
})

it('requires a successful reread when both saving and its state verification fail', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus)
    .mockResolvedValueOnce(defaultStatus)
    .mockRejectedValueOnce(new Error('private file path'))
    .mockResolvedValueOnce({ ...defaultStatus, enabled: true })
  vi.mocked(SetAutostartEnabled).mockRejectedValueOnce(new Error('private write detail'))
  show()

  await user.click(await screen.findByRole('switch', { name: '开机自启动' }))
  expect(await screen.findByText('无法读取开机自启动状态，请重新读取后再试。')).toBeTruthy()
  expect(screen.queryByRole('switch', { name: '开机自启动' })).toBeNull()
  expect(document.body.textContent).not.toContain('private file path')
  expect(document.body.textContent).not.toContain('private write detail')

  await user.click(screen.getByRole('button', { name: '重新读取' }))
  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(toggle.hasAttribute('disabled')).toBe(false)
})

it('keeps autostart unavailable on unsupported systems', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, supported: false })
  show()

  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(screen.getByText('当前系统不支持开机自启动。')).toBeTruthy()
  await user.click(toggle)
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('offers re-enabling to repair an outdated registration', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, needsRepair: true })
  show()

  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  expect(screen.getByText(/已有启动项需要修复/)).toBeTruthy()
  await user.click(toggle)

  expect(SetAutostartEnabled).toHaveBeenCalledWith(true)
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect(screen.queryByText(/已有启动项需要修复/)).toBeNull()
})

it('can remove an outdated registration without enabling autostart', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, needsRepair: true })
  show()

  await user.click(await screen.findByRole('button', { name: '清理启动项' }))
  expect(SetAutostartEnabled).toHaveBeenCalledWith(false)
  await waitFor(() => expect(screen.queryByText(/已有启动项需要修复/)).toBeNull())
  expect(screen.getByRole('switch', { name: '开机自启动' }).getAttribute('aria-checked')).toBe(
    'false',
  )
})

it('shows safe read failures and retries through the backend', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockRejectedValueOnce(new Error('private executable path'))
  show()

  expect(await screen.findByText('无法读取开机自启动状态，请重新读取后再试。')).toBeTruthy()
  expect(screen.queryByRole('switch', { name: '开机自启动' })).toBeNull()
  expect(document.body.textContent).not.toContain('private executable path')
  await user.click(screen.getByRole('button', { name: '重新读取' }))

  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  expect(toggle.hasAttribute('disabled')).toBe(false)
  expect(GetAutostartStatus).toHaveBeenCalledTimes(2)
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('prevents repeated writes while a registration change is pending', async () => {
  const user = userEvent.setup()
  let resolveChange: (status: typeof defaultStatus) => void = () => {}
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, needsRepair: true })
  vi.mocked(SetAutostartEnabled).mockImplementationOnce(
    () =>
      new CancellablePromise((resolve) => {
        resolveChange = resolve
      }),
  )
  show()

  const toggle = await screen.findByRole('switch', { name: '开机自启动' })
  await user.click(toggle)
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(toggle.getAttribute('aria-busy')).toBe('true')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  const clean = screen.getByRole('button', { name: '清理启动项' })
  expect(clean.hasAttribute('disabled')).toBe(true)
  expect(screen.getByText('正在更新开机自启动状态…')).toBeTruthy()
  await user.click(toggle)
  await user.click(clean)
  expect(SetAutostartEnabled).toHaveBeenCalledTimes(1)
  expect(GetAutostartStatus).toHaveBeenCalledTimes(1)

  resolveChange({ ...defaultStatus, enabled: true })
  await waitFor(() => {
    expect(toggle.getAttribute('aria-checked')).toBe('true')
    expect(toggle.hasAttribute('disabled')).toBe(false)
  })
})

it('shows the main window on launch by default', async () => {
  show()

  const toggle = await screen.findByRole('switch', { name: '启动后隐藏窗口' })
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  expect(screen.getByText('启动后不显示主窗口，仅保留系统托盘入口')).toBeTruthy()
  expect(SetStartupVisibility).not.toHaveBeenCalled()
})

it('keeps a stored hidden launch through the settings binding', async () => {
  vi.mocked(GetStartupVisibility).mockResolvedValueOnce('hide')
  show()

  const toggle = await screen.findByRole('switch', { name: '启动后隐藏窗口' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(SetStartupVisibility).not.toHaveBeenCalled()
})

it('hides the window on the next launch and restores it afterwards', async () => {
  const user = userEvent.setup()
  show()

  const toggle = await screen.findByRole('switch', { name: '启动后隐藏窗口' })
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  await user.click(toggle)
  expect(SetStartupVisibility).toHaveBeenCalledWith('hide')
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '已开启启动后隐藏窗口，下次启动生效。',
  )

  toggle.focus()
  await user.keyboard(' ')
  expect(SetStartupVisibility).toHaveBeenLastCalledWith('show')
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '已关闭启动后隐藏窗口，下次启动显示主窗口。',
  )
})

it('rereads the stored preference after a failed write and reports it safely', async () => {
  const user = userEvent.setup()
  vi.mocked(GetStartupVisibility)
    .mockResolvedValueOnce('show')
    .mockRejectedValueOnce(new Error('private preference path'))
  vi.mocked(SetStartupVisibility).mockRejectedValueOnce(new Error('private write detail'))
  show()

  await user.click(await screen.findByRole('switch', { name: '启动后隐藏窗口' }))
  expect(await screen.findByText('无法读取启动后隐藏窗口设置，请重新读取后再试。')).toBeTruthy()
  expect(screen.queryByRole('switch', { name: '启动后隐藏窗口' })).toBeNull()
  expect(document.body.textContent).not.toContain('private preference path')
  expect(document.body.textContent).not.toContain('private write detail')

  vi.mocked(GetStartupVisibility).mockResolvedValueOnce('hide')
  await user.click(screen.getByRole('button', { name: '重新读取' }))
  const toggle = await screen.findByRole('switch', { name: '启动后隐藏窗口' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
})

it('keeps the hidden launch available when autostart cannot be read', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockRejectedValueOnce(new Error('private registry path'))
  show()

  expect(await screen.findByText('无法读取开机自启动状态，请重新读取后再试。')).toBeTruthy()
  const toggle = await screen.findByRole('switch', { name: '启动后隐藏窗口' })
  expect(toggle.hasAttribute('disabled')).toBe(false)
  await user.click(toggle)
  expect(SetStartupVisibility).toHaveBeenCalledWith('hide')
})
