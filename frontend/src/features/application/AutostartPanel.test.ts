import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { CancellablePromise } from '@wailsio/runtime'
import { beforeEach, expect, it, vi } from 'vitest'
import {
  GetAutostartStatus,
  SetAutostartEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/autostartservice'
import { createAppQueryClient } from '@/app/queryClient'
import { mountToastHost } from '@/test/toastHost'
import AutostartPanel from './AutostartPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/autostartservice', () => ({
  GetAutostartStatus: vi.fn(),
  SetAutostartEnabled: vi.fn(),
}))

const defaultStatus = { enabled: false, supported: true, needsRepair: false }

function show() {
  mountToastHost()
  return render(AutostartPanel, {
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
})

it('loads an existing enabled registration and refreshes changes made outside the app', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, enabled: true })
  show()

  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(screen.getByText('登录桌面会话后自动启动 dan-tick')).toBeTruthy()
  expect(SetAutostartEnabled).not.toHaveBeenCalled()

  await user.click(screen.getByRole('button', { name: '刷新状态' }))
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('enables and disables autostart using the confirmed system state', async () => {
  const user = userEvent.setup()
  show()

  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
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

  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
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

  await user.click(await screen.findByRole('switch', { name: '启用开机自启动' }))
  expect(await screen.findByText('无法读取开机自启动状态，请重新读取后再试。')).toBeTruthy()
  expect(screen.queryByRole('switch')).toBeNull()
  expect(document.body.textContent).not.toContain('private file path')
  expect(document.body.textContent).not.toContain('private write detail')

  await user.click(screen.getByRole('button', { name: '重新读取' }))
  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(toggle.hasAttribute('disabled')).toBe(false)
})

it('keeps autostart unavailable on unsupported systems', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, supported: false })
  show()

  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(screen.getByText('当前系统不支持开机自启动。')).toBeTruthy()
  await user.click(toggle)
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('offers re-enabling to repair an outdated registration', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, needsRepair: true })
  show()

  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
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
  expect(screen.getByRole('switch').getAttribute('aria-checked')).toBe('false')
})

it('shows safe read failures and retries through the backend', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockRejectedValueOnce(new Error('private executable path'))
  show()

  expect(screen.getByRole('status').textContent).toContain('正在读取开机自启动状态')
  expect(screen.queryByRole('switch')).toBeNull()
  expect(await screen.findByText('无法读取开机自启动状态，请重新读取后再试。')).toBeTruthy()
  expect(document.body.textContent).not.toContain('private executable path')
  await user.click(screen.getByRole('button', { name: '重新读取' }))

  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
  expect(toggle.hasAttribute('disabled')).toBe(false)
  expect(GetAutostartStatus).toHaveBeenCalledTimes(2)
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('prevents repeated writes and refreshing while a registration change is pending', async () => {
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

  const toggle = await screen.findByRole('switch', { name: '启用开机自启动' })
  await user.click(toggle)
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(toggle.getAttribute('aria-busy')).toBe('true')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  const refresh = screen.getByRole('button', { name: '刷新状态' })
  const clean = screen.getByRole('button', { name: '清理启动项' })
  expect(refresh.hasAttribute('disabled')).toBe(true)
  expect(clean.hasAttribute('disabled')).toBe(true)
  expect(screen.getByText('正在更新开机自启动状态…')).toBeTruthy()
  await user.click(toggle)
  await user.click(clean)
  await user.click(refresh)
  expect(SetAutostartEnabled).toHaveBeenCalledTimes(1)
  expect(GetAutostartStatus).toHaveBeenCalledTimes(1)

  resolveChange({ ...defaultStatus, enabled: true })
  await waitFor(() => {
    expect(toggle.getAttribute('aria-checked')).toBe('true')
    expect(toggle.hasAttribute('disabled')).toBe(false)
  })
})
