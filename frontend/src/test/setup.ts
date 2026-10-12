import { cleanup, configure } from '@testing-library/vue'
import { afterEach, vi } from 'vitest'

// The first render in a file also waits for Vite to transform the whole lazy
// route chunk. On a cold cache that exceeds the 1s Testing Library default, so
// keep the wait window about machine speed instead of asserting on it.
configure({ asyncUtilTimeout: 5000 })

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  localStorage.clear()
  window.history.replaceState(null, '', '/#/')
  document.documentElement.removeAttribute('data-theme')
})
