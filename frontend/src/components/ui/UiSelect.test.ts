import { fireEvent, render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'
import UiSelect from './UiSelect.vue'

const options = [
  { value: 'single', label: '单次发送' },
  { value: 'loop', label: '循环发送' },
  { value: 'retired', label: '已停用的类型', disabled: true },
]

function renderSelect(props: Record<string, unknown> = {}) {
  return render(UiSelect, {
    props: { options, placeholder: '请选择任务类型', ...props },
    attrs: { id: 'task-kind', 'aria-describedby': 'task-kind-error' },
  })
}

async function openSelect(user: ReturnType<typeof userEvent.setup>, trigger: HTMLElement) {
  await user.click(trigger)
  return await screen.findByRole('listbox')
}

it('keeps the caller id, description and placeholder on the trigger', () => {
  renderSelect()

  const trigger = screen.getByRole('combobox')
  expect(trigger.id).toBe('task-kind')
  expect(trigger.getAttribute('aria-describedby')).toBe('task-kind-error')
  expect(trigger.getAttribute('aria-expanded')).toBe('false')
  expect(trigger.textContent).toContain('请选择任务类型')
})

it('shows the label of the current value instead of the placeholder', () => {
  renderSelect({ modelValue: 'loop' })

  const trigger = screen.getByRole('combobox')
  expect(trigger.textContent).toContain('循环发送')
  expect(trigger.textContent).not.toContain('请选择任务类型')
})

it('opens from the keyboard and picks the highlighted option', async () => {
  const user = userEvent.setup()
  const view = renderSelect()
  const trigger = screen.getByRole('combobox')

  trigger.focus()
  await user.keyboard('{Enter}')
  const listbox = await screen.findByRole('listbox')
  expect(trigger.getAttribute('aria-expanded')).toBe('true')
  expect(trigger.getAttribute('aria-controls')).toBe(listbox.id)

  await user.keyboard('{ArrowDown}{Enter}')

  expect(view.emitted('update:modelValue')).toEqual([[options[1]?.value]])
  await waitFor(() => expect(screen.queryByRole('listbox')).toBeNull())
})

it('marks the current option as selected and closes after picking another one', async () => {
  const user = userEvent.setup()
  const view = renderSelect({ modelValue: 'single' })
  const trigger = screen.getByRole('combobox')

  const listbox = await openSelect(user, trigger)
  expect(
    within(listbox).getByRole('option', { name: '单次发送' }).getAttribute('aria-selected'),
  ).toBe('true')
  expect(
    within(listbox).getByRole('option', { name: '循环发送' }).getAttribute('aria-selected'),
  ).toBe('false')

  await user.click(within(listbox).getByRole('option', { name: '循环发送' }))

  expect(view.emitted('update:modelValue')).toEqual([['loop']])
  await waitFor(() => expect(screen.queryByRole('listbox')).toBeNull())
})

it('keeps the value when the list is dismissed with Escape or an outside click', async () => {
  const user = userEvent.setup()
  const view = renderSelect({ modelValue: 'single' })
  const trigger = screen.getByRole('combobox')

  await openSelect(user, trigger)
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('listbox')).toBeNull())

  await openSelect(user, trigger)
  // The open layer blocks pointer events on the rest of the page, exactly like
  // the native popup does: the first outside press only dismisses the list.
  await fireEvent.pointerDown(document.body)
  await waitFor(() => expect(screen.queryByRole('listbox')).toBeNull())

  expect(view.emitted('update:modelValue')).toBeUndefined()
  expect(trigger.textContent).toContain('单次发送')
})

it('does not open while disabled', async () => {
  const user = userEvent.setup()
  renderSelect({ disabled: true })

  const trigger = screen.getByRole('combobox')
  expect(trigger.hasAttribute('disabled')).toBe(true)

  await user.click(trigger)

  expect(screen.queryByRole('listbox')).toBeNull()
})

it('does not select a disabled option', async () => {
  const user = userEvent.setup()
  const view = renderSelect()
  const trigger = screen.getByRole('combobox')

  const listbox = await openSelect(user, trigger)
  const disabledOption = within(listbox).getByRole('option', { name: '已停用的类型' })
  expect(disabledOption.getAttribute('aria-disabled')).toBe('true')

  await user.click(disabledOption)

  expect(view.emitted('update:modelValue')).toBeUndefined()
})

it('reports blur so a form can mark the field as touched', async () => {
  const view = renderSelect()

  await fireEvent.blur(screen.getByRole('combobox'))

  expect(view.emitted('blur')).toHaveLength(1)
})
