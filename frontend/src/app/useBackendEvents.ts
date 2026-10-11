import { onMounted, onUnmounted } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { Events } from '@wailsio/runtime'
import { operationKeys } from '@/features/operations/queries'

// A single subscription at app-shell lifetime. Never keep Event payloads as
// business state; refresh only the affected authoritative Go queries.
export function useBackendEvents() {
  const client = useQueryClient()
  let unsubscribe: (() => void)[] = []

  onMounted(() => {
    unsubscribe = [
      Events.On('dan-tick:account-changed', () => {
        void client.invalidateQueries({ queryKey: operationKeys.account })
      }),
      Events.On('dan-tick:execution-changed', () => {
        void client.invalidateQueries({ queryKey: operationKeys.executions })
      }),
    ]
  })
  onUnmounted(() => {
    for (const off of unsubscribe) off()
    unsubscribe = []
  })
}
