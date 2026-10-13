import { fireEvent, render, screen } from '@testing-library/vue'
import { nextTick } from 'vue'
import { expect, it } from 'vitest'
import UiDialog from './UiDialog.vue'

function renderDialog(
  props: Record<string, unknown> = {},
  slots: Record<string, string> = {},
): ReturnType<typeof render> {
  return render(UiDialog, {
    props: { open: true, title: '添加直播间', description: '直播间信息由 B 站查询。', ...props },
    slots: { default: '<p>表单内容</p>', ...slots },
  })
}

it('renders a modal dialog with its title, description and close control', async () => {
  renderDialog()
  await nextTick()

  const dialog = screen.getByRole('dialog', { name: '添加直播间' })
  expect(dialog.getAttribute('aria-modal')).toBe('true')
  expect(dialog.textContent).toContain('表单内容')
  expect(dialog.textContent).toContain('直播间信息由 B 站查询。')
  expect(screen.getByRole('button', { name: '关闭添加直播间' })).toBeTruthy()
})

it('requests closing from the close control', async () => {
  const view = renderDialog()
  await nextTick()

  await fireEvent.click(screen.getByRole('button', { name: '关闭添加直播间' }))

  expect(view.emitted('update:open')).toEqual([[false]])
})

it('dismisses with Escape and reports the requested state', async () => {
  const view = renderDialog()
  await nextTick()

  await fireEvent.keyDown(screen.getByRole('dialog', { name: '添加直播间' }), { key: 'Escape' })

  expect(view.emitted('update:open')).toEqual([[false]])
})

it('keeps a busy dialog open and disables its close control', async () => {
  const view = renderDialog({ busy: true })
  await nextTick()

  expect(screen.getByRole<HTMLButtonElement>('button', { name: '关闭添加直播间' }).disabled).toBe(
    true,
  )
  await fireEvent.keyDown(screen.getByRole('dialog', { name: '添加直播间' }), { key: 'Escape' })

  expect(view.emitted('update:open')).toBeUndefined()
  expect(screen.getByRole('dialog', { name: '添加直播间' })).toBeTruthy()
})

it('lets a consumer take over the mount autofocus', async () => {
  renderDialog(
    {
      onOpenAutoFocus: (event: Event) => {
        event.preventDefault()
        document.getElementById('first-field')?.focus()
      },
    },
    { default: '<input id="first-field" aria-label="直播间 ID" />' },
  )
  await nextTick()

  expect(document.activeElement).toBe(screen.getByRole('textbox', { name: '直播间 ID' }))
})

it('hides the description node when no supporting text is provided', async () => {
  render(UiDialog, {
    props: { open: true, title: '添加直播间' },
    slots: { default: '<p>表单内容</p>' },
  })
  await nextTick()

  const dialog = screen.getByRole('dialog', { name: '添加直播间' })
  expect(dialog.textContent).toContain('表单内容')
  const describedBy = dialog.getAttribute('aria-describedby')
  expect(describedBy).not.toBeNull()
  expect(document.getElementById(describedBy ?? '')?.textContent).toBe('')
})
