import { onUnmounted, watch, type Ref } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { RefreshRoomMetadata } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import type { Room } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { configKeys } from './queries'

const refreshIntervalMs = 30 * 60 * 1000
const betweenRoomsMs = 5 * 1000

// One in-flight refresh at a time, with a pause between rooms. The Go service
// persists its last attempt and skips fresh entries, including after reopening
// the page. This queue only exists while the room page is mounted.
export function useRoomMetadataRefresh(rooms: Readonly<Ref<Room[]>>) {
  const client = useQueryClient()
  const pending = new Set<number>()
  const seen = new Set<number>()
  const queue: number[] = []
  let active = true
  let running = false
  let nextTimer: ReturnType<typeof setTimeout> | undefined

  async function processNext() {
    if (!active || running || queue.length === 0) return
    const id = queue.shift()
    if (id === undefined) return
    running = true
    try {
      const changed = await RefreshRoomMetadata(id)
      if (active && changed) {
        await client.invalidateQueries({ queryKey: configKeys.rooms })
      }
    } catch {
      // Refresh is best-effort; saved names remain usable during an outage.
    } finally {
      pending.delete(id)
      if (active) {
        nextTimer = setTimeout(() => {
          nextTimer = undefined
          running = false
          void processNext()
        }, betweenRoomsMs)
      }
    }
  }

  function enqueue(ids: number[]) {
    for (const id of ids) {
      if (!pending.has(id)) {
        pending.add(id)
        queue.push(id)
      }
    }
    void processNext()
  }

  watch(
    () => rooms.value.map((room) => room.id).join(','),
    () => {
      const newlyLoaded = rooms.value.filter((room) => !seen.has(room.id))
      for (const room of newlyLoaded) seen.add(room.id)
      enqueue(newlyLoaded.map((room) => room.id))
    },
    { immediate: true },
  )
  const interval = setInterval(() => enqueue(rooms.value.map((room) => room.id)), refreshIntervalMs)

  onUnmounted(() => {
    active = false
    clearInterval(interval)
    if (nextTimer !== undefined) clearTimeout(nextTimer)
    pending.clear()
    queue.length = 0
  })
}
