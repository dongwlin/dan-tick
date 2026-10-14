import { defineComponent, h } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { render, waitFor } from '@testing-library/vue'
import { expect, it, vi } from 'vitest'
import { Events } from '@wailsio/runtime'
import { createAppQueryClient } from './queryClient'
import { useBackendEvents } from './useBackendEvents'
import { operationKeys } from '@/features/operations/queries'
import { historyKeys } from '@/features/history/queries'
import { applicationKeys } from '@/features/application/queries'

it('invalidates only matching Query keys and detaches all listeners on unmount', async () => {
  const handlers = new Map<string, () => void>()
  const off = vi.fn()
  vi.spyOn(Events, 'On').mockImplementation((name, callback) => {
    handlers.set(name, () => callback({ name, data: undefined }))
    return off
  })
  const client = createAppQueryClient()
  const invalidated = vi.spyOn(client, 'invalidateQueries')
  const Observer = defineComponent({
    setup() {
      useBackendEvents()
      return () => h('div', 'observer')
    },
  })
  const view = render(Observer, {
    global: { plugins: [[VueQueryPlugin, { queryClient: client }]] },
  })
  expect(handlers.size).toBe(3)
  handlers.get('dan-tick:execution-changed')?.()
  await waitFor(() =>
    expect(invalidated).toHaveBeenCalledWith({ queryKey: operationKeys.executions }),
  )
  expect(invalidated).toHaveBeenCalledWith({ queryKey: historyKeys.all })
  expect(invalidated).toHaveBeenCalledWith({ queryKey: applicationKeys.overview })
  expect(invalidated).not.toHaveBeenCalledWith({ queryKey: operationKeys.account })
  handlers.get('dan-tick:account-changed')?.()
  await waitFor(() => expect(invalidated).toHaveBeenCalledWith({ queryKey: operationKeys.account }))
  handlers.get('dan-tick:application-changed')?.()
  await waitFor(() => expect(invalidated).toHaveBeenCalledWith({ queryKey: applicationKeys.all }))
  view.unmount()
  expect(off).toHaveBeenCalledTimes(3)
})
