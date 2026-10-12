import { render, screen } from '@testing-library/vue'
import { expect, it } from 'vitest'
import UiPageHeader from './UiPageHeader.vue'

it('renders the page heading, optional description and action area', () => {
  const view = render(UiPageHeader, {
    props: { title: '执行记录', description: '最近 30 天' },
    slots: { actions: '<button type="button">刷新</button>' },
  })

  expect(screen.getByRole('heading', { level: 1, name: '执行记录' })).toBeTruthy()
  expect(screen.getByText('最近 30 天')).toBeTruthy()
  expect(screen.getByRole('button', { name: '刷新' })).toBeTruthy()
  expect(view.container.querySelector('header')).toBeTruthy()
})

it('omits the action wrapper when there are no actions', () => {
  const view = render(UiPageHeader, { props: { title: '总览' } })
  expect(view.container.querySelectorAll('header > div')).toHaveLength(1)
})
