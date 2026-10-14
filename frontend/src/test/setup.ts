/// <reference types="node" />
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import { cleanup, configure } from '@testing-library/vue'
import { afterEach, vi } from 'vitest'
import type * as WailsRuntime from '@wailsio/runtime'
import { clearToasts } from '@/composables/useToast'

// Importing the runtime entry point starts native/browser IPC and drag timers.
// Keep DOM tests at the Binding boundary, while retaining the real pure helpers.
vi.mock('@wailsio/runtime', async () => {
  const runtimeEntry = pathToFileURL(
    createRequire(`${process.cwd()}/package.json`).resolve('@wailsio/runtime'),
  )
  const cancellable = (await import(new URL('./cancellable.js', runtimeEntry).href)) as Pick<
    typeof WailsRuntime,
    'CancellablePromise' | 'CancelError' | 'CancelledRejectionError'
  >
  const Create = (await import(
    new URL('./create.js', runtimeEntry).href
  )) as typeof WailsRuntime.Create

  return {
    ...cancellable,
    Create,
    Events: { On: vi.fn(() => vi.fn()) },
    Browser: { OpenURL: vi.fn().mockResolvedValue(undefined) },
    Window: {
      Minimise: vi.fn().mockResolvedValue(undefined),
      ToggleMaximise: vi.fn().mockResolvedValue(undefined),
      Close: vi.fn().mockResolvedValue(undefined),
    },
    Call: {
      ByID: vi.fn(() => {
        throw new Error('Mock the generated Wails Binding before calling it in a DOM test')
      }),
    },
  }
})

// The first render in a file also waits for Vite to transform the whole lazy
// route chunk. On a cold cache that exceeds the 1s Testing Library default, so
// keep the wait window about machine speed instead of asserting on it.
configure({ asyncUtilTimeout: 5000 })

afterEach(() => {
  cleanup()
  // Toast feedback lives in one shared slot, so a case must not inherit the
  // message another case raised.
  clearToasts()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  localStorage.clear()
  window.history.replaceState(null, '', '/#/')
  document.documentElement.removeAttribute('data-theme')
})
