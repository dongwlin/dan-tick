import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import App from './App.vue'
import { createAppRouter } from './router'

function renderAtHash(path: string) {
  window.history.replaceState(null, '', `/#${path}`)
  const router = createAppRouter()
  render(App, { global: { plugins: [router] } })
  return userEvent.setup()
}

describe('application navigation', () => {
  it('navigates from a deep link using the real Hash Router', async () => {
    const user = renderAtHash('/rooms')

    expect(await screen.findByRole('heading', { name: '直播间' })).toBeTruthy()
    await user.click(screen.getByRole('link', { name: '设置' }))

    expect(await screen.findByRole('heading', { name: '设置' })).toBeTruthy()
    expect(window.location.hash).toBe('#/settings')
  })

  it('offers a working return link on an unknown deep link', async () => {
    const user = renderAtHash('/does-not-exist')

    expect(await screen.findByRole('heading', { name: '页面不存在' })).toBeTruthy()
    await user.click(screen.getByRole('link', { name: '返回总览' }))

    expect(await screen.findByRole('heading', { name: '总览' })).toBeTruthy()
    expect(window.location.hash).toBe('#/')
  })

  it('highlights parent navigation for editing and detail routes', async () => {
    const user = renderAtHash('/tasks/123/edit')
    expect(await screen.findByRole('heading', { name: '编辑任务' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '弹幕任务' }).getAttribute('aria-current')).toBe('page')

    await user.click(screen.getByRole('link', { name: '执行记录' }))
    expect(await screen.findByRole('heading', { name: '执行记录' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '执行记录' }).getAttribute('aria-current')).toBe('page')
    await user.click(screen.getByRole('link', { name: '打开 B 站账号设置（尚未接入）' }))
    expect(await screen.findByRole('heading', { name: '设置' })).toBeTruthy()
  })
})
