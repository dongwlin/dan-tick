import { render, screen, waitFor, within } from '@testing-library/vue'
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
  return render(StartupPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
}

/** Reads resolve asynchronously, so a switch is usable only after its read. */
async function findReadySwitch(name: string) {
  const toggle = await screen.findByRole('switch', { name })
  await waitFor(() => expect(toggle.hasAttribute('disabled')).toBe(false))
  return toggle
}

function successToast() {
  return screen.findByRole('status', { name: '操作成功通知' })
}

beforeEach(() => {
  mountToastHost()
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

  const toggle = await findReadySwitch('开机自启')
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(screen.getByText('登录桌面会话后自动启动 dan-tick')).toBeTruthy()
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('enables and disables autostart using the confirmed system state', async () => {
  const user = userEvent.setup()
  show()

  const toggle = await findReadySwitch('开机自启')
  await user.click(toggle)
  expect(SetAutostartEnabled).toHaveBeenCalledWith(true)
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect((await successToast()).textContent).toContain('已开启开机自启')
  await waitFor(() => expect(toggle.hasAttribute('disabled')).toBe(false))

  toggle.focus()
  await user.keyboard(' ')
  expect(SetAutostartEnabled).toHaveBeenLastCalledWith(false)
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))
  expect((await successToast()).textContent).toContain('已关闭开机自启')
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

  const toggle = await findReadySwitch('开机自启')
  await user.click(toggle)
  await waitFor(() => expect(GetAutostartStatus).toHaveBeenCalledTimes(2))
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  await user.click(toggle)
  expect(SetAutostartEnabled).toHaveBeenCalledTimes(1)

  resolveRead({ ...defaultStatus, enabled: true })
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '更改开机自启未成功确认',
  )
  expect(toggle.hasAttribute('disabled')).toBe(false)
  expect(document.body.textContent).not.toContain('private registry path')
})

it('reports a failed write and a failed verification through the toast and rereads on the next visit', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus)
    .mockResolvedValueOnce(defaultStatus)
    .mockRejectedValueOnce(new Error('private file path'))
    .mockResolvedValueOnce({ ...defaultStatus, enabled: true })
  vi.mocked(SetAutostartEnabled).mockRejectedValueOnce(new Error('private write detail'))
  const view = show()

  await user.click(await findReadySwitch('开机自启'))
  await waitFor(() => expect(GetAutostartStatus).toHaveBeenCalledTimes(2))

  const toggle = screen.getByRole('switch', { name: '开机自启' })
  // The write and the verification both fail, so the single toast slot can end
  // up with either safe message; the card itself must stay in place either way.
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toMatch(
    /无法读取开机自启状态|更改开机自启未成功确认/,
  )
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(document.body.textContent).not.toContain('private file path')
  expect(document.body.textContent).not.toContain('private write detail')

  view.unmount()
  show()
  const reread = await findReadySwitch('开机自启')
  expect(reread.getAttribute('aria-checked')).toBe('true')
})

it('keeps autostart unavailable on unsupported systems', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, supported: false })
  show()

  const warning = await screen.findByRole('alert', { name: '操作警告通知' })
  expect(warning.textContent).toContain('当前系统不支持开机自启。')
  const toggle = screen.getByRole('switch', { name: '开机自启' })
  expect(toggle.hasAttribute('disabled')).toBe(true)
  await user.click(toggle)
  expect(SetAutostartEnabled).not.toHaveBeenCalled()
})

it('offers re-enabling to repair an outdated registration', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockResolvedValueOnce({ ...defaultStatus, needsRepair: true })
  show()

  const toggle = await findReadySwitch('开机自启')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  const warning = await screen.findByRole('alert', { name: '操作警告通知' })
  expect(warning.textContent).toContain('已有启动项需要修复，重新开启即可更新。')
  await user.click(toggle)

  expect(SetAutostartEnabled).toHaveBeenCalledWith(true)
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect(screen.queryByText(/已有启动项需要修复/)).toBeNull()
})

it('shows safe read failures and rereads on the next visit', async () => {
  vi.mocked(GetAutostartStatus).mockRejectedValueOnce(new Error('private executable path'))
  const view = show()

  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '无法读取开机自启状态，请重新进入设置页后再试。',
  )
  expect(screen.getByRole('switch', { name: '开机自启' }).hasAttribute('disabled')).toBe(true)
  expect(document.body.textContent).not.toContain('private executable path')

  view.unmount()
  show()
  expect((await findReadySwitch('开机自启')).getAttribute('aria-checked')).toBe('false')
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

  const toggle = await findReadySwitch('开机自启')
  await user.click(toggle)
  expect(toggle.hasAttribute('disabled')).toBe(true)
  expect(toggle.getAttribute('aria-busy')).toBe('true')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  expect((await screen.findByRole('status', { name: '操作提示通知' })).textContent).toContain(
    '正在更新开机自启状态…',
  )
  expect(screen.getByRole('switch', { name: '静默启动' }).hasAttribute('disabled')).toBe(true)
  await user.click(toggle)
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

  const toggle = await findReadySwitch('静默启动')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  expect(screen.getByText('启动后不显示主窗口，仅保留系统托盘入口')).toBeTruthy()
  expect(SetStartupVisibility).not.toHaveBeenCalled()
})

it('keeps a stored hidden launch through the settings binding', async () => {
  vi.mocked(GetStartupVisibility).mockResolvedValueOnce('hide')
  show()

  const toggle = await findReadySwitch('静默启动')
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(SetStartupVisibility).not.toHaveBeenCalled()
})

it('hides the window on the next launch and restores it afterwards', async () => {
  const user = userEvent.setup()
  show()

  const toggle = await findReadySwitch('静默启动')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  await user.click(toggle)
  expect(SetStartupVisibility).toHaveBeenCalledWith('hide')
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect((await successToast()).textContent).toContain('已开启静默启动，下次启动生效。')
  await waitFor(() => expect(toggle.hasAttribute('disabled')).toBe(false))

  toggle.focus()
  await user.keyboard(' ')
  expect(SetStartupVisibility).toHaveBeenLastCalledWith('show')
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))
  expect((await successToast()).textContent).toContain('已关闭静默启动，下次启动显示主窗口。')
})

it('rereads the stored preference after a failed write and reports it safely', async () => {
  const user = userEvent.setup()
  vi.mocked(GetStartupVisibility).mockResolvedValueOnce('show').mockResolvedValueOnce('hide')
  vi.mocked(SetStartupVisibility).mockRejectedValueOnce(new Error('private write detail'))
  show()

  const toggle = await findReadySwitch('静默启动')
  await user.click(toggle)
  await waitFor(() => expect(GetStartupVisibility).toHaveBeenCalledTimes(2))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '更改静默启动未成功确认',
  )
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  await waitFor(() => expect(toggle.hasAttribute('disabled')).toBe(false))
  expect(document.body.textContent).not.toContain('private write detail')
})

it('keeps the hidden launch available when autostart cannot be read', async () => {
  const user = userEvent.setup()
  vi.mocked(GetAutostartStatus).mockRejectedValueOnce(new Error('private registry path'))
  show()

  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '无法读取开机自启状态',
  )
  const toggle = await findReadySwitch('静默启动')
  await user.click(toggle)
  expect(SetStartupVisibility).toHaveBeenCalledWith('hide')
})

it('renders the startup card with the switches only, without inline actions or hints', async () => {
  show()

  const card = screen.getByRole('region', { name: '启动设置' })
  await waitFor(() => expect(within(card).getAllByRole('switch')).toHaveLength(2))
  expect(within(card).queryByRole('button')).toBeNull()
  expect(screen.queryByRole('button', { name: '刷新状态' })).toBeNull()
  expect(screen.queryByText(/下一次启动 dan-tick 时生效/)).toBeNull()
})
