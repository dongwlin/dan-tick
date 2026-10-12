import { VueQueryPlugin } from '@tanstack/vue-query'
import { fireEvent, render, screen } from '@testing-library/vue'
import { nextTick } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { State } from '@bindings/github.com/dongwlin/dan-tick/internal/account/models'
import {
  BeginQRLogin,
  CancelQRLogin,
  CheckAccount,
  ClearAccount,
  ConfirmQRAccount,
  PollQRLogin,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { createAppQueryClient } from '@/app/queryClient'
import AccountPanel from './AccountPanel.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice', () => ({
  CheckAccount: vi.fn(),
  ClearAccount: vi.fn(),
  BeginQRLogin: vi.fn(),
  PollQRLogin: vi.fn(),
  ConfirmQRAccount: vi.fn(),
  CancelQRLogin: vi.fn(),
}))

const qrImage = 'data:image/png;base64,fictional_qr_payload'

beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(CheckAccount).mockResolvedValue({ state: State.Unconfigured })
  vi.mocked(BeginQRLogin).mockResolvedValue({
    state: 'waiting',
    qrImage,
    expiresAt: Date.now() + 180000,
  })
  vi.mocked(PollQRLogin).mockResolvedValue({ state: 'waiting' })
  vi.mocked(CancelQRLogin).mockResolvedValue(undefined)
  vi.mocked(ConfirmQRAccount).mockResolvedValue(undefined)
  vi.mocked(ClearAccount).mockResolvedValue(undefined)
})

afterEach(() => {
  vi.useRealTimers()
})

async function renderPanel() {
  const client = createAppQueryClient()
  const view = render(AccountPanel, {
    global: { plugins: [[VueQueryPlugin, { queryClient: client }]] },
  })
  await screen.findByRole('button', { name: '扫码登录' })
  vi.useFakeTimers()
  await vi.advanceTimersByTimeAsync(0)
  return { ...view, client }
}

async function beginLogin() {
  await fireEvent.click(screen.getByRole('button', { name: '扫码登录' }))
  await nextTick()
}

async function poll() {
  await vi.advanceTimersByTimeAsync(2000)
  await nextTick()
}

it('replaces manual credentials with an in-app QR and refreshes the verified account after confirmation', async () => {
  vi.mocked(PollQRLogin)
    .mockResolvedValueOnce({ state: 'scanned' })
    .mockResolvedValueOnce({ state: 'confirmed' })
  const { client } = await renderPanel()
  expect(screen.queryByLabelText('SESSDATA')).toBeNull()
  expect(screen.queryByLabelText('bili_jct')).toBeNull()
  expect(screen.queryByRole('button', { name: '保存凭证' })).toBeNull()
  expect(document.querySelector('input[type="password"]')).toBeNull()

  await beginLogin()
  expect(screen.getByRole('img').getAttribute('src')).toBe(qrImage)
  expect(screen.getByText('等待扫码')).toBeTruthy()
  expect(vi.mocked(PollQRLogin)).not.toHaveBeenCalled()
  await poll()
  expect(screen.getByText('已扫码，请在手机上确认登录')).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
  vi.mocked(CheckAccount).mockResolvedValue({ state: State.LoggedIn, uid: '123456' })
  await poll()
  expect(screen.getByText('扫码登录成功')).toBeTruthy()
  expect(screen.getByText('已登录')).toBeTruthy()
  expect(vi.mocked(CheckAccount)).toHaveBeenCalledTimes(2)
  expect(vi.mocked(ConfirmQRAccount)).not.toHaveBeenCalled()
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data),
    ),
  ).not.toContain('fictional_qr_payload')
  expect(client.getMutationCache().getAll()).toHaveLength(0)
  expect(localStorage.length).toBe(0)
})

it('cancels the login and stops polling while preserving the existing account', async () => {
  await renderPanel()
  await beginLogin()
  await fireEvent.click(screen.getByRole('button', { name: '取消扫码' }))
  expect(screen.getByText('已取消扫码登录')).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
  expect(vi.mocked(CancelQRLogin)).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(6000)
  expect(vi.mocked(PollQRLogin)).not.toHaveBeenCalled()
  expect(vi.mocked(ClearAccount)).not.toHaveBeenCalled()
})

it('offers a new QR after expiry without automatically restarting login', async () => {
  vi.mocked(PollQRLogin).mockResolvedValueOnce({ state: 'expired' })
  await renderPanel()
  await beginLogin()
  await poll()
  expect(screen.getByText('二维码已过期，请重新获取')).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
  await vi.advanceTimersByTimeAsync(6000)
  expect(vi.mocked(PollQRLogin)).toHaveBeenCalledTimes(1)
  expect(vi.mocked(BeginQRLogin)).toHaveBeenCalledTimes(1)
  await fireEvent.click(screen.getByRole('button', { name: '重新获取二维码' }))
  expect(vi.mocked(BeginQRLogin)).toHaveBeenCalledTimes(2)
  expect(screen.getByRole('img').getAttribute('src')).toBe(qrImage)
})

it('requires an explicit account switch confirmation before replacing a different UID', async () => {
  vi.mocked(PollQRLogin).mockResolvedValueOnce({
    state: 'account_mismatch',
    candidateUID: '987654321',
  })
  await renderPanel()
  await beginLogin()
  await poll()
  expect(screen.getByText('扫码账号与当前绑定账号不同')).toBeTruthy()
  expect(screen.getByText(/本次扫码账号 UID：987654321/)).toBeTruthy()
  expect(screen.getByText(/现有任务将使用此账号发送弹幕/)).toBeTruthy()
  expect(vi.mocked(ConfirmQRAccount)).not.toHaveBeenCalled()
  expect(vi.mocked(CheckAccount)).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(6000)
  expect(vi.mocked(PollQRLogin)).toHaveBeenCalledTimes(1)
  await fireEvent.click(screen.getByRole('button', { name: '确认切换账号' }))
  await vi.advanceTimersByTimeAsync(0)
  expect(vi.mocked(ConfirmQRAccount)).toHaveBeenCalledTimes(1)
  expect(screen.getByText('扫码登录成功')).toBeTruthy()
  expect(vi.mocked(CheckAccount)).toHaveBeenCalledTimes(2)
})

it('allows cancelling an account mismatch without confirming or clearing credentials', async () => {
  vi.mocked(PollQRLogin).mockResolvedValueOnce({
    state: 'account_mismatch',
    candidateUID: '987654321',
  })
  await renderPanel()
  await beginLogin()
  await poll()
  await fireEvent.click(screen.getByRole('button', { name: '取消扫码' }))
  expect(screen.queryByText(/987654321/)).toBeNull()
  expect(screen.getByText('已取消扫码登录')).toBeTruthy()
  expect(vi.mocked(ConfirmQRAccount)).not.toHaveBeenCalled()
  expect(vi.mocked(ClearAccount)).not.toHaveBeenCalled()
})

it('shows safe login errors and offers retry without polling indefinitely', async () => {
  vi.mocked(PollQRLogin).mockRejectedValueOnce({
    message: '登录服务暂不可用，请稍后重新扫码或检查登录状态',
  })
  await renderPanel()
  await beginLogin()
  await poll()
  expect(screen.getByRole('alert').textContent).toBe(
    '登录服务暂不可用，请稍后重新扫码或检查登录状态',
  )
  expect(screen.getByRole('button', { name: '重试扫码登录' })).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
  await vi.advanceTimersByTimeAsync(6000)
  expect(vi.mocked(PollQRLogin)).toHaveBeenCalledTimes(1)
})

it('shows a preparation failure without keeping a QR image', async () => {
  vi.mocked(BeginQRLogin).mockRejectedValueOnce({
    message: '登录服务暂不可用，请稍后重新扫码或检查登录状态',
  })
  await renderPanel()
  await beginLogin()
  expect(screen.getByRole('alert').textContent).toBe(
    '登录服务暂不可用，请稍后重新扫码或检查登录状态',
  )
  expect(screen.queryByRole('img')).toBeNull()
  expect(screen.getByRole('button', { name: '重试扫码登录' })).toBeTruthy()
  expect(vi.mocked(PollQRLogin)).not.toHaveBeenCalled()
})

it('keeps the existing account when a confirmed switch cannot be securely saved', async () => {
  vi.mocked(PollQRLogin).mockResolvedValueOnce({
    state: 'account_mismatch',
    candidateUID: '987654321',
  })
  vi.mocked(ConfirmQRAccount).mockRejectedValueOnce({
    message: '系统安全凭证存储不可用，请检查系统密钥环',
  })
  await renderPanel()
  await beginLogin()
  await poll()
  await fireEvent.click(screen.getByRole('button', { name: '确认切换账号' }))
  expect(screen.getByRole('alert').textContent).toBe('系统安全凭证存储不可用，请检查系统密钥环')
  expect(screen.getByRole('button', { name: '重试扫码登录' })).toBeTruthy()
  expect(vi.mocked(CheckAccount)).toHaveBeenCalledTimes(1)
  expect(vi.mocked(ClearAccount)).not.toHaveBeenCalled()
})

it('cancels an active login when leaving the page and ignores a late poll result', async () => {
  let resolvePoll: ((result: { state: 'confirmed' }) => void) | undefined
  vi.mocked(PollQRLogin).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolvePoll = resolve
      }) as ReturnType<typeof PollQRLogin>,
  )
  const view = await renderPanel()
  await beginLogin()
  await poll()
  view.unmount()
  expect(vi.mocked(CancelQRLogin)).toHaveBeenCalledTimes(1)
  resolvePoll?.({ state: 'confirmed' })
  await vi.advanceTimersByTimeAsync(6000)
  expect(vi.mocked(CheckAccount)).toHaveBeenCalledTimes(1)
  expect(vi.mocked(PollQRLogin)).toHaveBeenCalledTimes(1)
})

it('does not revive a QR request that completes after cancellation', async () => {
  let resolveBegin:
    ((result: { state: 'waiting'; qrImage: string; expiresAt: number }) => void) | undefined
  vi.mocked(BeginQRLogin).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveBegin = resolve
      }) as ReturnType<typeof BeginQRLogin>,
  )
  await renderPanel()
  await beginLogin()
  expect(screen.getByText('正在准备二维码…')).toBeTruthy()
  await fireEvent.click(screen.getByRole('button', { name: '取消扫码' }))
  resolveBegin?.({ state: 'waiting', qrImage, expiresAt: Date.now() + 180000 })
  await vi.advanceTimersByTimeAsync(6000)
  expect(screen.getByText('已取消扫码登录')).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
  expect(vi.mocked(PollQRLogin)).not.toHaveBeenCalled()
})

it('guides legacy credentials to QR migration without deleting them', async () => {
  vi.mocked(CheckAccount).mockResolvedValue({ state: State.ReauthenticationRequired })
  await renderPanel()
  expect(screen.getByText('需要重新扫码')).toBeTruthy()
  expect(screen.getByText(/原凭证和任务会保留/)).toBeTruthy()
  expect(vi.mocked(ClearAccount)).not.toHaveBeenCalled()
  expect(vi.mocked(BeginQRLogin)).not.toHaveBeenCalled()
})
