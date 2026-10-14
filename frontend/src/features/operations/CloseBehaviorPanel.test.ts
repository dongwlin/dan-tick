import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { CancellablePromise } from '@wailsio/runtime'
import { beforeEach, expect, it, vi } from 'vitest'
import {
  GetCloseBehavior,
  SetCloseBehavior,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice'
import { createAppQueryClient } from '@/app/queryClient'
import { mountToastHost } from '@/test/toastHost'
import CloseBehaviorPanel from './CloseBehaviorPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice', () => ({
  GetCloseBehavior: vi.fn(),
  SetCloseBehavior: vi.fn(),
}))

const trayLabel = '最小化到系统托盘'
const quitLabel = '退出应用程序'

function show() {
  mountToastHost()
  return render(CloseBehaviorPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
}

/** The read settles asynchronously, so the select is usable only after it. */
async function findReadySelect() {
  const trigger = await screen.findByRole('combobox', { name: '关闭窗口时' })
  await waitFor(() => expect(trigger.hasAttribute('disabled')).toBe(false))
  return trigger
}

type User = ReturnType<typeof userEvent.setup>

async function pick(user: User, trigger: HTMLElement, label: string) {
  await user.click(trigger)
  const listbox = await screen.findByRole('listbox')
  await user.click(within(listbox).getByRole('option', { name: label }))
}

beforeEach(() => {
  vi.mocked(GetCloseBehavior).mockReset().mockResolvedValue('hide')
  vi.mocked(SetCloseBehavior).mockReset().mockResolvedValue(undefined)
})

it('shows the stored close behavior as one labelled row with both choices', async () => {
  const user = userEvent.setup()
  show()

  const card = screen.getByRole('region', { name: '窗口行为' })
  expect(card.querySelector('h2')?.textContent).toBe('窗口行为')
  const trigger = await findReadySelect()
  expect(trigger.textContent).toContain(trayLabel)

  const label = card.querySelector('label[for="close-behavior"]')
  expect(label?.textContent?.trim()).toBe('关闭窗口时')
  // The label sits beside the control in that row, not stacked above it.
  expect(label?.nextElementSibling).toBe(trigger)
  expect(label?.parentElement).toBe(trigger.parentElement)
  expect(label?.parentElement?.classList.contains('justify-between')).toBe(true)
  expect(card.querySelectorAll('input[type="radio"]')).toHaveLength(0)

  await user.click(trigger)
  const listbox = await screen.findByRole('listbox')
  expect(
    within(listbox)
      .getAllByRole('option')
      .map((option) => option.textContent),
  ).toEqual([trayLabel, quitLabel])
  expect(
    within(listbox).getByRole('option', { name: trayLabel }).getAttribute('aria-selected'),
  ).toBe('true')
})

it('hides the quit choice behind the tray wording and writes it through Go', async () => {
  const user = userEvent.setup()
  vi.mocked(SetCloseBehavior).mockImplementationOnce(() => {
    vi.mocked(GetCloseBehavior).mockResolvedValue('quit')
    return new CancellablePromise<void>((resolve) => resolve(undefined))
  })
  show()

  const card = screen.getByRole('region', { name: '窗口行为' })
  const trigger = await findReadySelect()
  expect(within(card).queryByText(/关闭时退出应用/)).toBeNull()
  const elements = card.querySelectorAll('*').length

  await pick(user, trigger, quitLabel)

  expect(SetCloseBehavior).toHaveBeenCalledWith('quit')
  await waitFor(() => expect(trigger.textContent).toContain(quitLabel))
  // The row is the only control and no feedback block joins the card, so it
  // neither grows nor shrinks around the write.
  expect(card.children).toHaveLength(2)
  expect(card.querySelectorAll('*').length).toBe(elements)
  expect(card.querySelectorAll('button')).toHaveLength(1)
  expect(within(card).queryByRole('alert')).toBeNull()
})

it('reads the stored behavior again when the write is not confirmed', async () => {
  const user = userEvent.setup()
  let rejectWrite: (reason: Error) => void = () => {}
  vi.mocked(SetCloseBehavior).mockImplementationOnce(
    () =>
      new CancellablePromise<void>((_resolve, reject) => {
        rejectWrite = reject
      }),
  )
  show()

  const trigger = await findReadySelect()
  await pick(user, trigger, quitLabel)

  // Optimistic: the picked option shows while the write runs.
  expect(trigger.textContent).toContain(quitLabel)
  expect(trigger.hasAttribute('disabled')).toBe(true)
  expect(trigger.getAttribute('aria-busy')).toBe('true')

  rejectWrite(new Error('private failure'))
  expect((await screen.findByRole('alert', { name: '操作失败通知' })).textContent).toContain(
    '更改窗口行为未成功确认，请确认当前选择后重试。',
  )
  // The reread restores what the backend actually holds.
  await waitFor(() => expect(trigger.textContent).toContain(trayLabel))
  await waitFor(() => expect(trigger.hasAttribute('disabled')).toBe(false))
  expect(GetCloseBehavior).toHaveBeenCalledTimes(2)
  expect(document.body.textContent).not.toContain('private failure')
})

it('does not write the option the backend already holds', async () => {
  const user = userEvent.setup()
  show()

  await pick(user, await findReadySelect(), trayLabel)

  expect(SetCloseBehavior).not.toHaveBeenCalled()
})

it('offers retry if the preference cannot be read', async () => {
  vi.mocked(GetCloseBehavior)
    .mockRejectedValueOnce(new Error('private failure'))
    .mockResolvedValue('hide')
  show()

  expect((await screen.findByRole('alert')).textContent).toContain('读取窗口偏好失败')
  await userEvent.setup().click(screen.getByRole('button', { name: '重试' }))
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: '关闭窗口时' }).textContent).toContain(trayLabel),
  )
})
