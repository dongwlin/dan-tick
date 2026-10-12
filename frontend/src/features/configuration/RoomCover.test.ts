import { VueQueryPlugin } from '@tanstack/vue-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { expect, it, vi } from 'vitest'
import { GetRoomCover } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import { createAppQueryClient } from '@/app/queryClient'
import RoomCover from './RoomCover.vue'

vi.mock('@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice', () => ({
  GetRoomCover: vi.fn(),
}))

function renderCover(index = 0) {
  return render(RoomCover, {
    props: { roomId: 3, platformRoomId: '300', index },
    global: { plugins: [[VueQueryPlugin, { queryClient: createAppQueryClient() }]] },
  })
}

it('shows the real cover after loading and hides the placeholder', async () => {
  vi.mocked(GetRoomCover).mockReset().mockResolvedValue('https://i0.hdslb.com/bfs/live/cover.jpg')
  const view = renderCover()
  expect(screen.getByText('直播间 · 300')).toBeTruthy()
  await waitFor(() =>
    expect(view.container.querySelector('img')?.getAttribute('src')).toBe(
      'https://i0.hdslb.com/bfs/live/cover.jpg',
    ),
  )
  const image = view.container.querySelector('img')
  expect(image?.getAttribute('referrerpolicy')).toBe('no-referrer')
  if (!image) throw new Error('missing cover image')
  await fireEvent.load(image)
  expect(screen.queryByText('直播间 · 300')).toBeNull()
  expect(GetRoomCover).toHaveBeenCalledWith(3)
})

it('keeps the original cover placeholder for absent, rejected or broken images', async () => {
  vi.mocked(GetRoomCover).mockReset().mockResolvedValue('https://i0.hdslb.com/bfs/live/cover.jpg')
  const view = renderCover(1)
  await waitFor(() => expect(view.container.querySelector('img')).toBeTruthy())
  const image = view.container.querySelector('img')
  if (!image) throw new Error('missing cover image')
  await fireEvent.error(image)
  expect(view.container.querySelector('img')).toBeNull()
  expect(screen.getByText('直播间 · 300')).toBeTruthy()
  expect(
    screen.getByText('直播间 · 300').parentElement?.classList.contains('bg-surface-muted'),
  ).toBe(true)
  view.unmount()

  vi.mocked(GetRoomCover).mockResolvedValue('')
  const withoutCover = renderCover(2)
  await waitFor(() => expect(GetRoomCover).toHaveBeenCalledTimes(2))
  expect(screen.getByText('直播间 · 300')).toBeTruthy()
  expect(withoutCover.container.querySelector('img')).toBeNull()
  withoutCover.unmount()

  vi.mocked(GetRoomCover).mockRejectedValue(new Error('offline'))
  const offline = renderCover()
  await waitFor(() => expect(GetRoomCover).toHaveBeenCalledTimes(3))
  expect(screen.getByText('直播间 · 300')).toBeTruthy()
  expect(offline.container.querySelector('img')).toBeNull()
})
