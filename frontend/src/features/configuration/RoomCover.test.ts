import { fireEvent, render } from '@testing-library/vue'
import { expect, it } from 'vitest'
import RoomCover from './RoomCover.vue'

const imageURL = 'https://i0.hdslb.com/bfs/live/cover.jpg'

function renderCover(coverUrl = imageURL, index = 0) {
  return render(RoomCover, {
    props: { coverUrl, index },
  })
}

it('shows the saved cover after loading and resets when the artwork changes', async () => {
  const view = renderCover()
  expect(view.container.querySelector('[aria-hidden="true"] svg')).toBeTruthy()
  expect(view.container.textContent?.trim()).toBe('')
  const image = view.container.querySelector('img')
  expect(image?.getAttribute('src')).toBe(imageURL)
  expect(image?.getAttribute('referrerpolicy')).toBe('no-referrer')
  if (!image) throw new Error('missing cover')
  await fireEvent.load(image)
  expect(view.container.querySelector('[aria-hidden="true"]')).toBeNull()
  await view.rerender({ coverUrl: 'https://i1.hdslb.com/bfs/live/new.jpg' })
  expect(view.container.querySelector('[aria-hidden="true"] svg')).toBeTruthy()
  expect(view.container.querySelector('img')?.getAttribute('src')).toContain('new.jpg')
})

it('keeps the original placeholder if the cover is absent or broken', async () => {
  const view = renderCover(imageURL, 1)
  const image = view.container.querySelector('img')
  if (!image) throw new Error('missing cover')
  await fireEvent.error(image)
  expect(view.container.querySelector('img')).toBeNull()
  expect(view.container.querySelector('[aria-hidden="true"] svg')).toBeTruthy()
  expect(view.container.firstElementChild?.classList.contains('bg-surface-muted')).toBe(true)
  expect(view.container.textContent?.trim()).toBe('')
  await view.rerender({ coverUrl: '' })
  expect(view.container.querySelector('img')).toBeNull()
  expect(view.container.querySelector('[aria-hidden="true"] svg')).toBeTruthy()
})
