import { render, screen } from '@testing-library/vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { expect, it } from 'vitest'
import UiButton from './UiButton.vue'

it('uses shared variants and button sizes, with a visible loading state', () => {
  render(UiButton, {
    props: { variant: 'danger', size: 'default', loading: true },
    slots: { default: '删除中…' },
  })

  const button = screen.getByRole('button', { name: '删除中…' })
  expect(button.getAttribute('type')).toBe('button')
  expect(button.classList.contains('ui-button')).toBe(true)
  expect(button.classList.contains('ui-button--default')).toBe(true)
  expect(button.classList.contains('bg-danger-solid')).toBe(true)
  expect(button.hasAttribute('disabled')).toBe(true)
  expect(button.getAttribute('aria-busy')).toBe('true')
})

it('acts as a submit button when a form needs one', () => {
  render(UiButton, {
    props: { type: 'submit', variant: 'primary' },
    slots: { default: '保存任务' },
  })

  expect(screen.getByRole('button', { name: '保存任务' }).getAttribute('type')).toBe('submit')
})

it('prevents navigating while a link-style button is disabled', async () => {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div>首页</div>' } },
      { path: '/tasks', component: { template: '<div>任务</div>' } },
    ],
  })
  await router.push('/')
  await router.isReady()
  render(UiButton, {
    props: { to: '/tasks', disabled: true, size: 'small', variant: 'ghost' },
    slots: { default: '查看任务' },
    global: { plugins: [router] },
  })

  const link = screen.getByRole('link', { name: '查看任务' })
  expect(link.classList.contains('ui-button--small')).toBe(true)
  expect(link.getAttribute('aria-disabled')).toBe('true')
  link.click()
  expect(router.currentRoute.value.path).toBe('/')
})
