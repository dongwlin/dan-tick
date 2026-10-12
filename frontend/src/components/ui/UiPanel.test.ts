import { render, screen } from '@testing-library/vue'
import { expect, it } from 'vitest'
import UiPanel from './UiPanel.vue'

it('renders an acrylic panel surface by default', () => {
  render(UiPanel, {
    props: { 'aria-label': '概览' },
    slots: { default: '面板内容' },
  })

  const panel = screen.getByRole('region', { name: '概览' })
  expect(panel.tagName).toBe('SECTION')
  expect(panel.classList.contains('acrylic')).toBe(true)
  expect(panel.classList.contains('rounded-[14px]')).toBe(true)
  expect(panel.classList.contains('p-5')).toBe(true)
  expect(panel.textContent).toBe('面板内容')
})

it('can render a solid surface, another element and no built-in padding', () => {
  render(UiPanel, {
    props: { as: 'article', variant: 'solid', padded: false },
    attrs: { 'data-testid': 'panel' },
  })

  const panel = screen.getByTestId('panel')
  expect(panel.tagName).toBe('ARTICLE')
  expect(panel.classList.contains('bg-surface-solid')).toBe(true)
  expect(panel.classList.contains('acrylic')).toBe(false)
  expect(panel.classList.contains('p-5')).toBe(false)
})
