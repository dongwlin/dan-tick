import { render, screen } from '@testing-library/vue'
import { h } from 'vue'
import { expect, it } from 'vitest'
import UiField from './UiField.vue'

interface FieldSlotProps {
  id: string
  ariaInvalid: true | undefined
  describedBy: string
}

function control({ id, ariaInvalid, describedBy }: FieldSlotProps) {
  return h('input', { id, 'aria-invalid': ariaInvalid, 'aria-describedby': describedBy })
}

function renderField(props: Record<string, unknown> = {}) {
  return render(UiField, { props: { label: '任务名称', ...props }, slots: { default: control } })
}

it('associates a real label with the control and links error and hint', () => {
  renderField({ id: 'task-name', error: '请输入任务名称', description: '每行一条，空行自动忽略' })

  const input = screen.getByLabelText('任务名称')
  expect(input.id).toBe('task-name')
  expect(input.getAttribute('aria-invalid')).toBe('true')
  expect(input.getAttribute('aria-describedby')).toBe('task-name-error task-name-description')
  expect(screen.getByText('请输入任务名称').id).toBe('task-name-error')
  expect(screen.getByText('每行一条，空行自动忽略').id).toBe('task-name-description')
})

it('omits aria-invalid while the field is valid', () => {
  renderField({ id: 'task-name' })

  const input = screen.getByLabelText('任务名称')
  expect(input.hasAttribute('aria-invalid')).toBe(false)
  expect(input.getAttribute('aria-describedby')).toBe('task-name-error')
})

it('generates a distinct control id when none is given', () => {
  render({
    setup: () => () =>
      h('div', [
        h(UiField, { label: '任务名称' }, { default: control }),
        h(UiField, { label: '发送队列' }, { default: control }),
      ]),
  })

  const name = screen.getByLabelText('任务名称')
  const messages = screen.getByLabelText('发送队列')
  expect(name.id).not.toBe('')
  expect(messages.id).not.toBe(name.id)
})

it('keeps caller layout classes on the field wrapper', () => {
  const { container } = render({
    setup: () => () =>
      h(
        UiField,
        { label: '任务名称', id: 'task-name', class: 'md:col-span-2' },
        { default: control },
      ),
  })

  expect(container.firstElementChild?.classList.contains('md:col-span-2')).toBe(true)
})

it('labels a control group with a fieldset legend', () => {
  renderField({ label: '选择星期', group: true, id: 'task-weekdays', error: '请至少选择一天' })

  const group = screen.getByRole('group')
  expect(group.tagName).toBe('FIELDSET')
  expect(group.querySelector('legend')?.textContent).toContain('选择星期')
  expect(screen.getByText('请至少选择一天').id).toBe('task-weekdays-error')
  expect(screen.getByRole('textbox').getAttribute('aria-describedby')).toBe('task-weekdays-error')
})
