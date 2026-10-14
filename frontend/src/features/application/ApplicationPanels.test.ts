import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { CancellablePromise } from '@wailsio/runtime'
import { beforeEach, expect, it, vi } from 'vitest'
import type { Component } from 'vue'
import { createAppQueryClient } from '@/app/queryClient'
import { mountToastHost } from '@/test/toastHost'
import {
  GetPreferences,
  SetDefaultTimezone,
  SetSchedulingEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice'
import {
  ExportDiagnostics,
  GetDiagnosticsStatus,
  OpenDataDirectory,
  OpenLogDirectory,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/diagnosticsservice'
import SchedulingPanel from './SchedulingPanel.vue'
import DiagnosticsPanel from './DiagnosticsPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice', () => ({
  GetPreferences: vi.fn(),
  SetDefaultTimezone: vi.fn(),
  SetSchedulingEnabled: vi.fn(),
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/diagnosticsservice', () => ({
  GetDiagnosticsStatus: vi.fn(),
  OpenLogDirectory: vi.fn(),
  OpenDataDirectory: vi.fn(),
  ExportDiagnostics: vi.fn(),
}))

function show(component: Component) {
  mountToastHost()
  return render(component, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
}

beforeEach(() => {
  vi.mocked(GetPreferences).mockReset().mockResolvedValue({
    defaultTimezone: 'Asia/Shanghai',
    schedulingEnabled: true,
  })
  vi.mocked(GetDiagnosticsStatus).mockReset().mockResolvedValue({
    directory: '/tmp/dan-tick/logs',
    dataDirectory: '/tmp/dan-tick/data',
    degraded: false,
    warning: '',
  })
  vi.mocked(SetDefaultTimezone).mockReset().mockResolvedValue(undefined)
  vi.mocked(SetSchedulingEnabled).mockReset().mockResolvedValue(undefined)
  vi.mocked(OpenLogDirectory).mockReset().mockResolvedValue(undefined)
  vi.mocked(OpenDataDirectory).mockReset().mockResolvedValue(undefined)
  vi.mocked(ExportDiagnostics)
    .mockReset()
    .mockResolvedValue({ cancelled: false, filename: 'safe.zip' })
})

it('updates scheduling through Go and preserves authoritative state after failures', async () => {
  const user = userEvent.setup()
  vi.mocked(SetSchedulingEnabled).mockImplementationOnce((enabled) => {
    vi.mocked(GetPreferences).mockResolvedValue({
      defaultTimezone: 'Asia/Shanghai',
      schedulingEnabled: enabled,
    })
    return new CancellablePromise<void>((resolve) => resolve())
  })
  show(SchedulingPanel)
  const card = screen.getByRole('region', { name: '调度设置' })
  const initialChildCount = card.children.length
  const toggle = await screen.findByRole('switch', { name: /启用自动调度/ })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(toggle.querySelector('[data-state="checked"]')).toBeTruthy()
  await user.click(screen.getByText('启用自动调度'))
  expect(SetSchedulingEnabled).toHaveBeenCalledWith(false)
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '已暂停新的自动调度',
  )
  expect(card.children).toHaveLength(initialChildCount)
  expect(within(card).queryByText(/已暂停新的自动调度/)).toBeNull()
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))

  vi.mocked(SetDefaultTimezone).mockRejectedValueOnce(new Error('private DB path'))
  const input = screen.getByRole<HTMLInputElement>('textbox', { name: /默认时区/ })
  await user.clear(input)
  await user.type(input, 'Invalid/Timezone')
  await user.click(screen.getByRole('button', { name: '保存默认时区' }))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '默认时区保存失败',
  )
  expect(input.value).toBe('Invalid/Timezone')
  expect(card.children).toHaveLength(initialChildCount)
  expect(within(card).queryByText(/默认时区保存失败/)).toBeNull()
  expect(document.body.textContent).not.toContain('private DB path')

  await user.clear(input)
  await user.type(input, 'Europe/Berlin')
  await user.click(screen.getByRole('button', { name: '保存默认时区' }))
  await waitFor(() => expect(SetDefaultTimezone).toHaveBeenCalledWith('Europe/Berlin'))
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '默认时区已保存',
  )
  expect(card.children).toHaveLength(initialChildCount)
})

it('flips the scheduling switch optimistically and restores the stored value after a failed update', async () => {
  const user = userEvent.setup()
  let rejectChange: (reason: Error) => void = () => {}
  vi.mocked(SetSchedulingEnabled).mockImplementationOnce(
    () =>
      new CancellablePromise<void>((_resolve, reject) => {
        rejectChange = reject
      }),
  )
  show(SchedulingPanel)

  const toggle = await screen.findByRole('switch', { name: /启用自动调度/ })
  toggle.focus()
  await user.keyboard(' ')

  expect(SetSchedulingEnabled).toHaveBeenCalledWith(false)
  // Optimistic: the requested value shows while the write runs, and the switch
  // stays locked so no second write can race it.
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))
  await waitFor(() => expect(toggle.hasAttribute('disabled')).toBe(true))
  await user.click(toggle)
  expect(SetSchedulingEnabled).toHaveBeenCalledTimes(1)
  rejectChange(new Error('private failure'))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '更改调度状态未成功确认',
  )
  expect(
    within(screen.getByRole('region', { name: '调度设置' })).queryByText(/更改调度状态未成功确认/),
  ).toBeNull()
  // The reread restores what the backend actually holds.
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'))
  expect(toggle.hasAttribute('disabled')).toBe(false)
  expect(document.body.textContent).not.toContain('private failure')
})

it('opens local directories and exports with result feedback without the explanatory notice', async () => {
  const user = userEvent.setup()
  show(DiagnosticsPanel)
  const card = screen.getByRole('region', { name: '数据与日志' })
  const initialChildCount = card.children.length
  expect(screen.queryByText(/不包含数据库、账号凭证或弹幕正文/)).toBeNull()
  expect(await within(card).findByText('/tmp/dan-tick/data')).toBeTruthy()
  expect(within(card).getByText('/tmp/dan-tick/logs')).toBeTruthy()
  expect(within(card).queryByText(/任务、设置和执行记录保存在本机 SQLite 中/)).toBeNull()
  expect(within(card).queryByText(/日志与执行历史分别保存/)).toBeNull()
  await user.click(screen.getByRole('button', { name: '打开日志目录' }))
  await waitFor(() => expect(OpenLogDirectory).toHaveBeenCalledOnce())
  await user.click(screen.getByRole('button', { name: '打开数据目录' }))
  await waitFor(() => expect(OpenDataDirectory).toHaveBeenCalledOnce())
  vi.mocked(ExportDiagnostics).mockResolvedValueOnce({ cancelled: true, filename: '' })
  await user.click(screen.getByRole('button', { name: /导出诊断包/ }))
  expect((await screen.findByRole('status', { name: '操作提示通知' })).textContent).toContain(
    '已取消导出。',
  )
  vi.mocked(ExportDiagnostics).mockRejectedValueOnce(new Error('secret should not appear'))
  await user.click(screen.getByRole('button', { name: /导出诊断包/ }))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '诊断包导出失败或未通过安全检查',
  )
  expect(document.body.textContent).not.toContain('secret should not appear')
  await user.click(screen.getByRole('button', { name: /导出诊断包/ }))
  expect((await screen.findByRole('status', { name: '操作成功通知' })).textContent).toContain(
    '诊断包已保存为 safe.zip',
  )
  expect(card.children).toHaveLength(initialChildCount)
  expect(within(card).queryByText(/诊断包已保存为/)).toBeNull()
})
