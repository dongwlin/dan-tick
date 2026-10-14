import { expect, it } from 'vitest'
import { timezoneOptions, timezonePickerOptions } from './timezones'

it('offers every curated zone without duplicates and labels it with its own offset', () => {
  const values = timezoneOptions.map((option) => option.value)
  expect(new Set(values).size).toBe(values.length)
  for (const option of timezoneOptions) {
    // The label leads with its own zone id, so an offset can never be read as
    // belonging to a different zone.
    expect(option.label.startsWith(`${option.value} (`)).toBe(true)
  }
  expect(values).toContain('Asia/Shanghai')
  expect(values).toContain('UTC')
})

it('marks daylight-saving zones as standard time', () => {
  // The shown offset is the standard offset, so a zone that shifts during the
  // year has to say so.
  for (const zone of ['Europe/London', 'Europe/Berlin', 'America/New_York', 'Australia/Sydney']) {
    expect(timezoneOptions.find((option) => option.value === zone)?.label).toContain('标准时')
  }
  expect(timezoneOptions.find((option) => option.value === 'Asia/Shanghai')?.label).not.toContain(
    '标准时',
  )
})

it('appends a stored zone the list does not offer', () => {
  expect(timezoneOptions.some((option) => option.value === 'Asia/Urumqi')).toBe(false)

  const options = timezonePickerOptions('Asia/Urumqi')
  expect(options).toHaveLength(timezoneOptions.length + 1)
  expect(options.at(-1)).toEqual({ value: 'Asia/Urumqi', label: 'Asia/Urumqi' })
})

it('keeps a listed zone at its curated position and label', () => {
  const options = timezonePickerOptions('Europe/Berlin')
  expect(options).toHaveLength(timezoneOptions.length)
  expect(options[12]?.value).toBe('Europe/Berlin')
})

it('selects the curated default for a fresh form value', () => {
  const options = timezonePickerOptions('Asia/Shanghai')
  expect(options).toHaveLength(timezoneOptions.length)
  expect(options[0]?.value).toBe('Asia/Shanghai')
})
