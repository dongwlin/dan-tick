import { render, screen } from '@testing-library/vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { expect, it } from 'vitest'
import UiIconButton from './UiIconButton.vue'

it('provides an accessible name and a 44px action target', () => {
  render(UiIconButton, {
    props: { label: '删除任务', variant: 'danger' },
    slots: { default: '<svg aria-hidden="true" />' },
  })

  const button = screen.getByRole('button', { name: '删除任务' })
  expect(button.classList.contains('size-11')).toBe(true)
  expect(button.classList.contains('text-soft-danger-fg')).toBe(true)
  expect(button.getAttribute('type')).toBe('button')
})

it('renders links with their navigation semantics and blocks disabled navigation', async () => {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div>首页</div>' } },
      { path: '/history', component: { template: '<div>历史</div>' } },
    ],
  })
  await router.push('/')
  await router.isReady()
  render(UiIconButton, {
    props: { label: '查看执行记录', to: '/history' },
    global: { plugins: [router] },
  })

  const link = screen.getByRole('link', { name: '查看执行记录' })
  expect(link.getAttribute('href')).toBe('/history')

  await router.push('/')
  render(UiIconButton, {
    props: { label: '查看执行记录', to: '/history', disabled: true },
    global: { plugins: [router] },
  })
  const disabled = screen.getAllByRole('link', { name: '查看执行记录' })[1]
  disabled.click()
  await router.isReady()
  expect(router.currentRoute.value.path).toBe('/')
})
