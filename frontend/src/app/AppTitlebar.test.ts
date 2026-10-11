import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import AppTitlebar from './AppTitlebar.vue'

const { minimise, toggleMaximise, close, quit } = vi.hoisted(() => ({
  minimise: vi.fn(),
  toggleMaximise: vi.fn(),
  close: vi.fn(),
  quit: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Window: { Minimise: minimise, ToggleMaximise: toggleMaximise, Close: close },
}))
vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice', () => ({
  Quit: quit,
}))

beforeEach(() => {
  vi.stubGlobal('_wails', { environment: { OS: 'linux' } })
  minimise.mockResolvedValue(undefined)
  toggleMaximise.mockResolvedValue(undefined)
  close.mockResolvedValue(undefined)
  quit.mockResolvedValue(undefined)
})

it('routes titlebar controls to the desktop window', async () => {
  const user = userEvent.setup()
  render(AppTitlebar)
  await user.click(screen.getByRole('button', { name: '最小化窗口' }))
  await user.click(screen.getByRole('button', { name: '最大化或还原窗口' }))
  await user.click(screen.getByRole('button', { name: '关闭窗口' }))
  await user.click(screen.getByRole('button', { name: '退出应用' }))
  expect(minimise).toHaveBeenCalledOnce()
  expect(toggleMaximise).toHaveBeenCalledOnce()
  expect(close).toHaveBeenCalledOnce()
  expect(quit).toHaveBeenCalledOnce()
  expect(screen.queryByRole('alert')).toBeNull()
})

it('explains unavailable browser controls and desktop failures', async () => {
  const user = userEvent.setup()
  render(AppTitlebar)
  vi.stubGlobal('_wails', undefined)
  await user.click(screen.getByRole('button', { name: '最小化窗口' }))
  expect(screen.getByRole('alert').textContent).toContain('浏览器预览无法控制窗口')
  expect(minimise).not.toHaveBeenCalled()

  vi.stubGlobal('_wails', { environment: { OS: 'windows' } })
  toggleMaximise.mockRejectedValueOnce(new Error('private native error'))
  await user.click(screen.getByRole('button', { name: '最大化或还原窗口' }))
  expect((await screen.findByRole('alert')).textContent).toBe('窗口操作失败，请重试。')
})
