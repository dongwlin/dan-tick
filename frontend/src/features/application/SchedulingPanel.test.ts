import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { CancellablePromise } from '@wailsio/runtime'
import { beforeEach, expect, it, vi } from 'vitest'
import {
  GetPreferences,
  SetDefaultTimezone,
  SetSchedulingEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice'
import { createAppQueryClient } from '@/app/queryClient'
import { timezoneOptions } from '@/lib/timezones'
import { mountToastHost } from '@/test/toastHost'
import SchedulingPanel from './SchedulingPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice', () => ({
  GetPreferences: vi.fn(),
  SetDefaultTimezone: vi.fn(),
  SetSchedulingEnabled: vi.fn(),
}))

function storedPreferences(defaultTimezone = 'Asia/Shanghai') {
  return { defaultTimezone, schedulingEnabled: true }
}

function show() {
  mountToastHost()
  return render(SchedulingPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
}

/** The read settles asynchronously, so the select is usable only after it. */
async function findReadyTimezone() {
  const trigger = await screen.findByRole('combobox', { name: '默认时区' })
  await waitFor(() => expect(trigger.hasAttribute('disabled')).toBe(false))
  return trigger
}

type User = ReturnType<typeof userEvent.setup>

async function openTimezoneList(user: User, trigger: HTMLElement) {
  await user.click(trigger)
  return await screen.findByRole('listbox')
}

async function pickTimezone(user: User, trigger: HTMLElement, label: string) {
  const listbox = await openTimezoneList(user, trigger)
  await user.click(within(listbox).getByRole('option', { name: label }))
}

function successToast() {
  return screen.findByRole('status', { name: '操作成功通知' })
}

const tokyoLabel = 'Asia/Tokyo (UTC+9) · 日本'
const utcLabel = 'UTC (UTC+0) · 协调世界时'

beforeEach(() => {
  vi.mocked(GetPreferences).mockReset().mockResolvedValue(storedPreferences())
  vi.mocked(SetDefaultTimezone).mockReset().mockResolvedValue(undefined)
  vi.mocked(SetSchedulingEnabled).mockReset().mockResolvedValue(undefined)
})

it('offers the default timezone as a select with the stored zone and common zones', async () => {
  const user = userEvent.setup()
  show()

  const trigger = await findReadyTimezone()
  expect(trigger.textContent).toContain('Asia/Shanghai (UTC+8) · 中国大陆')

  const listbox = await openTimezoneList(user, trigger)
  const labels = within(listbox)
    .getAllByRole('option')
    .map((option) => option.textContent)
  expect(labels).toEqual(timezoneOptions.map((option) => option.label))
  expect(within(listbox).getByRole('option', { name: /Europe\/Berlin/ })).toBeTruthy()
})

it('places the zone control beside the label in one row', async () => {
  show()

  const card = screen.getByRole('region', { name: '调度设置' })
  const trigger = await findReadyTimezone()
  const label = card.querySelector('label[for="default-timezone"]')
  const row = trigger.parentElement
  expect(label?.textContent?.trim()).toBe('默认时区')
  // The select is the label's next sibling in that row, not stacked under it.
  expect(label?.nextElementSibling).toBe(trigger)
  expect(label?.parentElement).toBe(row)
  expect(row?.classList.contains('items-center')).toBe(true)
  expect(row?.classList.contains('justify-between')).toBe(true)
})

it('keeps a default timezone that is not in the list selectable instead of hiding it', async () => {
  // Asia/Urumqi is a real IANA zone that this list does not offer, so the stored
  // value has to be appended; otherwise the control would read as empty.
  expect(timezoneOptions.some((option) => option.value === 'Asia/Urumqi')).toBe(false)
  vi.mocked(GetPreferences).mockResolvedValue(storedPreferences('Asia/Urumqi'))
  const user = userEvent.setup()
  show()

  const trigger = await findReadyTimezone()
  expect(trigger.textContent).toContain('Asia/Urumqi')

  const listbox = await openTimezoneList(user, trigger)
  const options = within(listbox).getAllByRole('option')
  expect(options).toHaveLength(timezoneOptions.length + 1)
  expect(options.at(-1)?.textContent).toBe('Asia/Urumqi')
  expect(options.at(-1)?.getAttribute('aria-selected')).toBe('true')

  // The appended zone stays selectable, and picking another zone still writes.
  await user.click(within(listbox).getByRole('option', { name: utcLabel }))
  await waitFor(() => expect(SetDefaultTimezone).toHaveBeenCalledWith('UTC'))
})

it('re-reads the stored zone after a successful write so the backend stays authoritative', async () => {
  const user = userEvent.setup()
  vi.mocked(GetPreferences)
    .mockResolvedValueOnce(storedPreferences())
    .mockResolvedValueOnce(storedPreferences('Asia/Tokyo'))
  vi.mocked(SetDefaultTimezone).mockImplementationOnce((value) => {
    // Simulate the backend accepting the write even if a concurrent re-read of
    // the old value landed first.
    vi.mocked(GetPreferences).mockResolvedValue(storedPreferences(value))
    return new CancellablePromise<void>((resolve) => resolve())
  })
  show()

  const trigger = await findReadyTimezone()
  await pickTimezone(user, trigger, tokyoLabel)

  await waitFor(() => expect(GetPreferences).toHaveBeenCalledTimes(2))
  expect(trigger.textContent).toContain(tokyoLabel)
  expect((await successToast()).textContent).toContain('默认时区已保存')
})

it('saves the picked timezone as soon as it changes, without a save button', async () => {
  const user = userEvent.setup()
  vi.mocked(SetDefaultTimezone).mockImplementationOnce((value) => {
    vi.mocked(GetPreferences).mockResolvedValue(storedPreferences(value))
    return new CancellablePromise<void>((resolve) => resolve())
  })
  show()

  const trigger = await findReadyTimezone()
  await pickTimezone(user, trigger, tokyoLabel)

  await waitFor(() => expect(SetDefaultTimezone).toHaveBeenCalledWith('Asia/Tokyo'))
  expect((await successToast()).textContent).toContain('默认时区已保存')
  await waitFor(() => expect(trigger.textContent).toContain(tokyoLabel))
})

it('shows the picked zone immediately and prevents a second write while one is pending', async () => {
  const user = userEvent.setup()
  let resolveWrite: () => void = () => {}
  vi.mocked(SetDefaultTimezone).mockImplementationOnce(
    () =>
      new CancellablePromise<void>((resolve) => {
        resolveWrite = () => resolve()
      }),
  )
  show()

  const trigger = await findReadyTimezone()
  await pickTimezone(user, trigger, tokyoLabel)

  expect(SetDefaultTimezone).toHaveBeenCalledTimes(1)
  expect(trigger.textContent).toContain(tokyoLabel)
  expect(trigger.hasAttribute('disabled')).toBe(true)
  expect(trigger.getAttribute('aria-busy')).toBe('true')
  expect((await screen.findByRole('status', { name: '操作提示通知' })).textContent).toContain(
    '正在保存默认时区',
  )

  await user.click(trigger)
  expect(screen.queryByRole('listbox')).toBeNull()
  expect(SetDefaultTimezone).toHaveBeenCalledTimes(1)

  resolveWrite()
  await waitFor(() => expect(trigger.hasAttribute('disabled')).toBe(false))
  expect((await successToast()).textContent).toContain('默认时区已保存')
})

it('restores the stored zone and reports safely when the write is not confirmed', async () => {
  const user = userEvent.setup()
  let rejectWrite: (reason: Error) => void = () => {}
  vi.mocked(GetPreferences)
    .mockResolvedValueOnce(storedPreferences())
    .mockResolvedValueOnce(storedPreferences())
  vi.mocked(SetDefaultTimezone).mockImplementationOnce(
    () =>
      new CancellablePromise<void>((_resolve, reject) => {
        rejectWrite = reject
      }),
  )
  show()

  const trigger = await findReadyTimezone()
  await pickTimezone(user, trigger, tokyoLabel)
  // Optimistic: the picked zone shows while the write runs.
  expect(trigger.textContent).toContain(tokyoLabel)
  expect(trigger.hasAttribute('disabled')).toBe(true)

  rejectWrite(new Error('private DB path'))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '默认时区保存失败',
  )
  // The re-read restores what the backend actually holds.
  await waitFor(() => expect(trigger.textContent).toContain('Asia/Shanghai'))
  await waitFor(() => expect(trigger.hasAttribute('disabled')).toBe(false))
  expect(document.body.textContent).not.toContain('private DB path')
})

it('keeps the card to the switch and the select, with feedback only in the toast', async () => {
  const user = userEvent.setup()
  vi.mocked(SetDefaultTimezone).mockImplementationOnce((value) => {
    vi.mocked(GetPreferences).mockResolvedValue(storedPreferences(value))
    return new CancellablePromise<void>((resolve) => resolve())
  })
  show()

  const card = screen.getByRole('region', { name: '调度设置' })
  await findReadyTimezone()
  expect(card.children).toHaveLength(2)
  // The switch and the select are the only controls in the card; the removed
  // timezone action must not leave a button (or its label) behind.
  expect([...card.querySelectorAll('button')].map((button) => button.getAttribute('role'))).toEqual(
    ['switch', 'combobox'],
  )
  expect(within(card).queryByText('保存默认时区')).toBeNull()
  expect(within(card).queryByText(/使用 IANA 时区名称/)).toBeNull()
  const elements = card.querySelectorAll('*').length

  await pickTimezone(user, screen.getByRole('combobox', { name: '默认时区' }), utcLabel)
  expect((await successToast()).textContent).toContain('默认时区已保存')
  expect(card.children).toHaveLength(2)
  expect(card.querySelectorAll('*').length).toBe(elements)
  expect(card.contains(screen.getByRole('status', { name: '操作成功通知' }))).toBe(false)
})

it('turns the scheduling switch off through Go and keeps the card stable', async () => {
  const user = userEvent.setup()
  vi.mocked(SetSchedulingEnabled).mockImplementationOnce((enabled) => {
    vi.mocked(GetPreferences).mockResolvedValue({
      defaultTimezone: 'Asia/Shanghai',
      schedulingEnabled: enabled,
    })
    return new CancellablePromise<void>((resolve) => resolve())
  })
  show()

  const card = screen.getByRole('region', { name: '调度设置' })
  const toggle = await screen.findByRole('switch', { name: /启用自动调度/ })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  await user.click(screen.getByText('启用自动调度'))
  expect(SetSchedulingEnabled).toHaveBeenCalledWith(false)
  expect((await successToast()).textContent).toContain('已暂停新的自动调度')
  expect(card.children).toHaveLength(2)
  expect(within(card).queryByText(/已暂停新的自动调度/)).toBeNull()
  await waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('false'))
})
