import { computed, onScopeDispose, readonly, shallowRef } from 'vue'
import {
  BeginQRLogin,
  CancelQRLogin,
  ConfirmQRAccount,
  PollQRLogin,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/operationsservice'
import { bindingError } from '@/features/configuration/queries'

export type QRLoginState =
  | 'idle'
  | 'preparing'
  | 'waiting'
  | 'scanned'
  | 'expired'
  | 'confirmed'
  | 'account_mismatch'
  | 'cancelled'
  | 'failed'

export function useQRLogin(options: { onConfirmed: () => Promise<void> }) {
  // QR images stay within this mounted feature. They never enter Query caches,
  // shared stores, URLs, logs or persistent browser storage.
  const state = shallowRef<QRLoginState>('idle')
  const qrImage = shallowRef('')
  const candidateUID = shallowRef('')
  const errorMessage = shallowRef('')
  const beginning = shallowRef(false)
  const actionPending = shallowRef(false)
  const cancelPending = shallowRef(false)
  const active = computed(() =>
    ['preparing', 'waiting', 'scanned', 'account_mismatch'].includes(state.value),
  )
  const busy = computed(() => beginning.value || actionPending.value)
  let timer: ReturnType<typeof setTimeout> | undefined
  let generation = 0
  let disposed = false
  let sessionActive = false

  function clearTimer() {
    clearTimeout(timer)
    timer = undefined
  }

  function clearSessionView() {
    qrImage.value = ''
    candidateUID.value = ''
  }

  function current(attempt: number) {
    return !disposed && generation === attempt
  }

  async function confirmed() {
    state.value = 'confirmed'
    sessionActive = false
    actionPending.value = false
    clearSessionView()
    await options.onConfirmed()
  }

  function schedulePoll(attempt: number) {
    timer = setTimeout(() => {
      void poll(attempt)
    }, 2000)
  }

  async function poll(attempt: number) {
    if (!current(attempt)) return
    try {
      const result = await PollQRLogin()
      if (!current(attempt)) return
      if (result.state === 'confirmed') {
        await confirmed()
      } else if (result.state === 'waiting' || result.state === 'scanned') {
        state.value = result.state
        schedulePoll(attempt)
      } else if (result.state === 'account_mismatch') {
        state.value = 'account_mismatch'
        clearSessionView()
        candidateUID.value = result.candidateUID ?? ''
      } else if (result.state === 'expired' || result.state === 'cancelled') {
        state.value = result.state
        clearSessionView()
        sessionActive = false
      } else {
        throw new Error('Unexpected QR login state')
      }
    } catch (error) {
      if (!current(attempt)) return
      state.value = 'failed'
      clearSessionView()
      errorMessage.value = bindingError(error)
    }
  }

  async function begin() {
    if (disposed || busy.value || active.value) return
    const attempt = ++generation
    beginning.value = true
    sessionActive = true
    state.value = 'preparing'
    errorMessage.value = ''
    clearSessionView()
    try {
      const result = await BeginQRLogin()
      if (!current(attempt)) {
        // A request that finishes after cancel/unmount must not revive a session.
        await CancelQRLogin()
        return
      }
      qrImage.value = result.qrImage ?? ''
      state.value = 'waiting'
      schedulePoll(attempt)
    } catch (error) {
      if (!current(attempt)) return
      state.value = 'failed'
      clearSessionView()
      errorMessage.value = bindingError(error)
    } finally {
      beginning.value = false
    }
  }

  async function cancel() {
    if (disposed || actionPending.value) return
    const attempt = ++generation
    clearTimer()
    clearSessionView()
    actionPending.value = true
    cancelPending.value = true
    errorMessage.value = ''
    try {
      await CancelQRLogin()
      if (!current(attempt)) return
      sessionActive = false
      state.value = 'cancelled'
    } catch (error) {
      if (!current(attempt)) return
      state.value = 'failed'
      errorMessage.value = bindingError(error)
    } finally {
      actionPending.value = false
      cancelPending.value = false
    }
  }

  async function confirmAccount() {
    if (disposed || actionPending.value || state.value !== 'account_mismatch') return
    const attempt = generation
    actionPending.value = true
    errorMessage.value = ''
    try {
      await ConfirmQRAccount()
      if (current(attempt)) await confirmed()
    } catch (error) {
      if (!current(attempt)) return
      state.value = 'failed'
      clearSessionView()
      errorMessage.value = bindingError(error)
    } finally {
      actionPending.value = false
    }
  }

  onScopeDispose(() => {
    disposed = true
    generation++
    clearTimer()
    clearSessionView()
    if (sessionActive) void CancelQRLogin().catch(() => {})
  })

  return {
    state: readonly(state),
    qrImage: readonly(qrImage),
    candidateUID: readonly(candidateUID),
    errorMessage: readonly(errorMessage),
    busy,
    active,
    cancelPending: readonly(cancelPending),
    begin,
    cancel,
    confirmAccount,
  }
}
