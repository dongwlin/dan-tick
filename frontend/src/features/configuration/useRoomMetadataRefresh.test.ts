import { defineComponent, h, nextTick, shallowRef } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { render } from '@testing-library/vue'
import { afterEach, expect, it, vi } from 'vitest'
import { RefreshRoomMetadata } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import type { Room } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { createAppQueryClient } from '@/app/queryClient'
import { useRoomMetadataRefresh } from './useRoomMetadataRefresh'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  RefreshRoomMetadata: vi.fn(),
}))

afterEach(() => {
  vi.useRealTimers()
  vi.mocked(RefreshRoomMetadata).mockReset()
})

function room(id: number): Room {
  return {
    id,
    platformRoomId: String(id),
    title: '',
    anchorName: '',
    remark: '',
    coverUrl: '',
    taskCount: 0,
  }
}

function setup(ids: number[]) {
  const rooms = shallowRef(ids.map(room))
  const queryClient = createAppQueryClient()
  const Observer = defineComponent({
    setup() {
      useRoomMetadataRefresh(rooms)
      return () => h('div', 'rooms')
    },
  })
  const view = render(Observer, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
  return { rooms, queryClient, view }
}

it('stagger-refreshes loaded rooms with no concurrent requests and rechecks periodically', async () => {
  vi.useFakeTimers()
  const deferred: { resolve: (value: boolean) => void } = { resolve: () => {} }
  vi.mocked(RefreshRoomMetadata)
    .mockReset()
    .mockImplementationOnce(
      () =>
        new Promise<boolean>((resolve) => {
          deferred.resolve = resolve
        }) as ReturnType<typeof RefreshRoomMetadata>,
    )
    .mockResolvedValue(false)
  const { view } = setup([1, 2, 3])
  expect(RefreshRoomMetadata).toHaveBeenCalledTimes(1)
  expect(RefreshRoomMetadata).toHaveBeenNthCalledWith(1, 1)
  await vi.advanceTimersByTimeAsync(10000)
  expect(RefreshRoomMetadata).toHaveBeenCalledTimes(1)
  deferred.resolve(false)
  await vi.advanceTimersByTimeAsync(4999)
  expect(RefreshRoomMetadata).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(1)
  expect(RefreshRoomMetadata).toHaveBeenNthCalledWith(2, 2)
  await vi.advanceTimersByTimeAsync(5000)
  expect(RefreshRoomMetadata).toHaveBeenNthCalledWith(3, 3)
  view.unmount()
  await vi.advanceTimersByTimeAsync(30 * 60 * 1000)
  expect(RefreshRoomMetadata).toHaveBeenCalledTimes(3)
})

it('queues rooms loaded later, refreshes the list on changes and cancels on unmount', async () => {
  vi.useFakeTimers()
  vi.mocked(RefreshRoomMetadata).mockReset().mockResolvedValue(true)
  const { rooms, queryClient, view } = setup([1])
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  await vi.advanceTimersByTimeAsync(0)
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['rooms'] })
  rooms.value = [room(1), room(2)]
  await nextTick()
  await vi.advanceTimersByTimeAsync(5000)
  expect(RefreshRoomMetadata).toHaveBeenNthCalledWith(2, 2)
  await vi.advanceTimersByTimeAsync(30 * 60 * 1000)
  expect(RefreshRoomMetadata).toHaveBeenCalledTimes(4)
  view.unmount()
  await vi.advanceTimersByTimeAsync(30 * 60 * 1000)
  expect(RefreshRoomMetadata).toHaveBeenCalledTimes(4)
})
