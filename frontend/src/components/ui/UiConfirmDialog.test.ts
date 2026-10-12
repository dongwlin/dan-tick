import { fireEvent, render, screen } from '@testing-library/vue'
import { nextTick } from 'vue'
import { expect, it } from 'vitest'
import UiConfirmDialog from './UiConfirmDialog.vue'

function renderDialog(props: Record<string, unknown> = {}) {
  return render(UiConfirmDialog, {
    props: {
      open: true,
      title: '删除任务',
      description: '删除后无法恢复。',
      confirmLabel: '删除任务',
      cancelLabel: '取消',
      ...props,
    },
  })
}

it('opens as an Alert Dialog with focus on the safe action', async () => {
  renderDialog()
  await nextTick()

  const dialog = screen.getByRole('alertdialog', { name: '删除任务' })
  expect(dialog.getAttribute('aria-modal')).toBe('true')
  expect(dialog.textContent).toContain('删除后无法恢复。')
  expect(document.activeElement).toBe(screen.getByRole('button', { name: '取消' }))
})

it('prevents the backdrop mousedown from taking focus out of the dialog', async () => {
  renderDialog()
  await nextTick()

  const overlay = document.querySelector('.fixed.inset-0')
  expect(overlay).not.toBeNull()
  const event = new MouseEvent('mousedown', { bubbles: true, cancelable: true })
  expect(overlay?.dispatchEvent(event)).toBe(false)
  expect(event.defaultPrevented).toBe(true)
})

it('emits confirm and requests closing when the action button is pressed', async () => {
  const view = renderDialog()
  await nextTick()

  await fireEvent.click(screen.getByRole('button', { name: '删除任务' }))

  expect(view.emitted('confirm')).toHaveLength(1)
  expect(view.emitted('update:open')).toEqual([[false]])
})

it('emits cancel and requests closing when the cancel button is pressed', async () => {
  const view = renderDialog()
  await nextTick()

  await fireEvent.click(screen.getByRole('button', { name: '取消' }))

  expect(view.emitted('cancel')).toHaveLength(1)
  expect(view.emitted('update:open')).toEqual([[false]])
})

it('does not dismiss an Alert Dialog with Escape', async () => {
  const view = renderDialog()
  await nextTick()

  await fireEvent.keyDown(screen.getByRole('alertdialog'), { key: 'Escape' })

  expect(view.emitted('update:open')).toBeUndefined()
  expect(screen.getByRole('alertdialog')).toBeTruthy()
})

it('dismisses a regular Dialog with Escape and reports it as cancel', async () => {
  const view = renderDialog({ dismissible: true })
  await nextTick()

  await fireEvent.keyDown(screen.getByRole('dialog', { name: '删除任务' }), { key: 'Escape' })

  expect(view.emitted('cancel')).toHaveLength(1)
  expect(view.emitted('update:open')).toEqual([[false]])
})
