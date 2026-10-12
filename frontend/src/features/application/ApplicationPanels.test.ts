import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { CancellablePromise } from '@wailsio/runtime'
import { beforeEach, expect, it, vi } from 'vitest'
import type { Component } from 'vue'
import { createAppQueryClient } from '@/app/queryClient'
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
  const toggle = await screen.findByRole('switch', { name: /启用自动调度/ })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(toggle.querySelector('[data-state="checked"]')).toBeTruthy()
  await user.click(screen.getByText('启用自动调度'))
  expect(SetSchedulingEnabled).toHaveBeenCalledWith(false)
  expect(await screen.findByText(/已暂停新的自动调度/)).toBeTruthy()
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))

  vi.mocked(SetDefaultTimezone).mockRejectedValueOnce(new Error('private DB path'))
  const input = screen.getByRole<HTMLInputElement>('textbox', { name: /默认时区/ })
  await user.clear(input)
  await user.type(input, 'Invalid/Timezone')
  await user.click(screen.getByRole('button', { name: '保存默认时区' }))
  expect(await screen.findByText(/默认时区保存失败/)).toBeTruthy()
  expect(input.value).toBe('Invalid/Timezone')
  expect(document.body.textContent).not.toContain('private DB path')

  await user.clear(input)
  await user.type(input, 'Europe/Berlin')
  await user.click(screen.getByRole('button', { name: '保存默认时区' }))
  await waitFor(() => expect(SetDefaultTimezone).toHaveBeenCalledWith('Europe/Berlin'))
  expect(await screen.findByText(/默认时区已保存/)).toBeTruthy()
})

it('keeps the scheduling switch on after a failed update and supports keyboard activation', async () => {
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
  await waitFor(() => expect(toggle.hasAttribute('disabled')).toBe(true))
  await user.click(toggle)
  expect(SetSchedulingEnabled).toHaveBeenCalledTimes(1)
  rejectChange(new Error('private failure'))
  expect(await screen.findByText(/更改调度状态未成功确认/)).toBeTruthy()
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(toggle.hasAttribute('disabled')).toBe(false)
  expect(document.body.textContent).not.toContain('private failure')
})

it('opens local directories and exports with result feedback without the explanatory notice', async () => {
  const user = userEvent.setup()
  show(DiagnosticsPanel)
  expect(screen.queryByText(/不包含数据库、账号凭证或弹幕正文/)).toBeNull()
  await user.click(screen.getByRole('button', { name: '打开日志目录' }))
  await waitFor(() => expect(OpenLogDirectory).toHaveBeenCalledOnce())
  await user.click(screen.getByRole('button', { name: '打开数据目录' }))
  await waitFor(() => expect(OpenDataDirectory).toHaveBeenCalledOnce())
  vi.mocked(ExportDiagnostics).mockResolvedValueOnce({ cancelled: true, filename: '' })
  await user.click(screen.getByRole('button', { name: /导出诊断包/ }))
  expect(await screen.findByText('已取消导出。')).toBeTruthy()
  vi.mocked(ExportDiagnostics).mockRejectedValueOnce(new Error('secret should not appear'))
  await user.click(screen.getByRole('button', { name: /导出诊断包/ }))
  expect(await screen.findByText(/诊断包导出失败或未通过安全检查/)).toBeTruthy()
  expect(document.body.textContent).not.toContain('secret should not appear')
  await user.click(screen.getByRole('button', { name: /导出诊断包/ }))
  expect(await screen.findByText(/诊断包已保存为 safe.zip/)).toBeTruthy()
})
