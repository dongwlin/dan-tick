import { render, screen, within } from '@testing-library/vue'
import { expect, it } from 'vitest'
import AboutPanel from './AboutPanel.vue'

it('shows a dev version and clear placeholders when build metadata is missing', () => {
  render(AboutPanel)

  const about = within(screen.getByRole('region', { name: '关于' }))
  expect(about.getByText('版本号').nextElementSibling?.textContent).toBe('dev')
  expect(about.getByText('构建时间').nextElementSibling?.textContent).toBe('未提供')
  expect(about.getByText('Git Commit').nextElementSibling?.textContent).toBe('未提供')
})

it('displays supplied metadata with local build time and a short commit', () => {
  render(AboutPanel, {
    props: {
      version: 'v1.2.3-a1b2c3d4',
      buildTime: '2026-10-12T09:47:00Z',
      commit: 'a1b2c3d4e5f67890',
    },
  })

  const about = within(screen.getByRole('region', { name: '关于' }))
  expect(about.getByText('版本号').nextElementSibling?.textContent).toBe('v1.2.3-a1b2c3d4')
  expect(about.getByText('构建时间').nextElementSibling?.textContent).toContain('2026')
  expect(about.getByText('Git Commit').nextElementSibling?.textContent).toBe('a1b2c3d4')
})

it('does not show malformed build times', () => {
  render(AboutPanel, { props: { buildTime: 'not-a-date' } })
  const about = within(screen.getByRole('region', { name: '关于' }))
  expect(about.getByText('构建时间').nextElementSibling?.textContent).toBe('未提供')
})

it('keeps the metadata placeholders visible while reporting a lookup failure', () => {
  render(AboutPanel, { props: { metadataUnavailable: true } })

  const about = within(screen.getByRole('region', { name: '关于' }))
  expect(about.getByRole('alert').textContent).toContain('无法读取构建信息')
  expect(about.getByText('版本号').nextElementSibling?.textContent).toBe('dev')
  expect(about.getByText('构建时间').nextElementSibling?.textContent).toBe('未提供')
  expect(about.getByText('Git Commit').nextElementSibling?.textContent).toBe('未提供')
})
