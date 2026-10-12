import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import { expect, it, vi } from 'vitest'
import { History } from 'lucide-vue-next'
import UiEmptyState from './UiEmptyState.vue'

it('renders an icon, explanation and a next-step action', async () => {
  const action = vi.fn()
  render(UiEmptyState, {
    props: {
      icon: History,
      title: '还没有执行记录',
      description: '运行任务后可在此查看结果。',
      actionLabel: '返回任务',
    },
    attrs: { onAction: action, 'data-testid': 'empty' },
  })

  const container = screen.getByTestId('empty')
  expect(container.classList.contains('justify-center')).toBe(true)
  expect(container.querySelector('svg')).not.toBeNull()
  expect(screen.getByText('还没有执行记录')).toBeTruthy()
  expect(screen.getByText('运行任务后可在此查看结果。')).toBeTruthy()
  await userEvent.setup().click(screen.getByRole('button', { name: '返回任务' }))
  expect(action).toHaveBeenCalledOnce()
})

it('uses the standard icon size in the compact vertical rhythm', () => {
  const view = render(UiEmptyState, {
    props: { icon: History, title: '还没有执行记录', fill: false },
  })

  const empty = view.container.firstElementChild
  expect(empty?.classList.contains('min-h-0')).toBe(true)
  expect(empty?.classList.contains('p-5')).toBe(true)
  expect(empty?.querySelector('svg')?.classList.contains('size-7')).toBe(true)
})

it('supports an in-app link as the next step', async () => {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div>首页</div>' } },
      { path: '/tasks', component: { template: '<div>任务</div>' } },
    ],
  })
  await router.push('/')
  await router.isReady()
  render(UiEmptyState, {
    props: {
      title: '还没有任务',
      actionLabel: '创建任务',
      actionTo: '/tasks',
    },
    global: { plugins: [router] },
  })

  const link = screen.getByRole('link', { name: '创建任务' })
  expect(link.getAttribute('href')).toBe('/tasks')
})
