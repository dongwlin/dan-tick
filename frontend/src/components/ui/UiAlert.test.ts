import { render, screen } from '@testing-library/vue'
import { expect, it } from 'vitest'
import UiAlert from './UiAlert.vue'

it('renders a semantic soft notice with the tone icon and accessible text', () => {
  render(UiAlert, {
    props: { tone: 'danger' },
    slots: { default: '删除失败，请重试。' },
  })

  const alert = screen.getByRole('alert')
  expect(alert.classList.contains('bg-soft-danger-bg')).toBe(true)
  expect(alert.classList.contains('border-soft-danger-border')).toBe(true)
  expect(alert.textContent).toBe('删除失败，请重试。')
  expect(alert.querySelector('svg')).not.toBeNull()
})

it('can render static inline text without a live region', () => {
  const view = render(UiAlert, {
    props: { tone: 'warning', variant: 'plain', role: '', showIcon: false },
    slots: { default: '自动调度已暂停。' },
  })

  const alert = view.container.firstElementChild
  expect(alert?.getAttribute('role')).toBeNull()
  expect(alert?.classList.contains('text-soft-warning-fg')).toBe(true)
  expect(alert?.querySelector('svg')).toBeNull()
})
