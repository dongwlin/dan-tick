import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import {
  CheckAccount,
  SaveAccount,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { createAppQueryClient } from '@/app/queryClient'
import AccountPanel from './AccountPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  CheckAccount: vi.fn(),
  SaveAccount: vi.fn(),
  ClearAccount: vi.fn(),
}))

it('preserves failed credential input, clears successful input, and never caches secrets', async () => {
  const user = userEvent.setup()
  const client = createAppQueryClient()
  vi.mocked(CheckAccount).mockResolvedValue({ state: State.Unconfigured })
  vi.mocked(SaveAccount)
    .mockRejectedValueOnce({ message: '系统安全凭证存储不可用，请检查系统密钥环' })
    .mockResolvedValue(undefined)
  render(AccountPanel, { global: { plugins: [[VueQueryPlugin, { queryClient: client }]] } })
  expect(await screen.findByText('未配置')).toBeTruthy()
  const session = screen.getByLabelText<HTMLInputElement>('SESSDATA')
  const csrf = screen.getByLabelText<HTMLInputElement>('bili_jct')
  await user.type(session, 'fictional_session_secret')
  await user.type(csrf, 'fictional_csrf_secret')
  await user.click(screen.getByRole('button', { name: '保存凭证' }))
  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    '系统安全凭证存储不可用，请检查系统密钥环',
  )
  expect(session.value).toBe('fictional_session_secret')
  expect(csrf.value).toBe('fictional_csrf_secret')

  await user.click(screen.getByRole('button', { name: '保存凭证' }))
  await waitFor(() => expect(session.value).toBe(''))
  expect(csrf.value).toBe('')
  expect(vi.mocked(SaveAccount)).toHaveBeenCalledWith({
    sessdata: 'fictional_session_secret',
    biliJct: 'fictional_csrf_secret',
  })
  expect(JSON.stringify(client.getMutationCache().getAll())).not.toContain('secret')
  expect(localStorage.length).toBe(0)
})
