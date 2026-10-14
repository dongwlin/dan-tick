/**
 * The timezone picker used by the default-timezone setting and the task editor.
 *
 * Go validates every stored value with `time.LoadLocation`, so the list only
 * needs to cover the zones this audience realistically schedules for; a zone
 * written elsewhere is appended instead of being dropped.
 *
 * Offsets are the zones' standard offsets, not "today's" offset: a zone that
 * observes daylight saving is one hour further ahead for part of the year.
 */
export interface TimezoneOption {
  value: string
  label: string
}

export const timezoneOptions: readonly TimezoneOption[] = [
  { value: 'Asia/Shanghai', label: 'Asia/Shanghai (UTC+8) · 中国大陆' },
  { value: 'Asia/Hong_Kong', label: 'Asia/Hong_Kong (UTC+8) · 中国香港' },
  { value: 'Asia/Macau', label: 'Asia/Macau (UTC+8) · 中国澳门' },
  { value: 'Asia/Taipei', label: 'Asia/Taipei (UTC+8) · 中国台北' },
  { value: 'Asia/Tokyo', label: 'Asia/Tokyo (UTC+9) · 日本' },
  { value: 'Asia/Seoul', label: 'Asia/Seoul (UTC+9) · 韩国' },
  { value: 'Asia/Singapore', label: 'Asia/Singapore (UTC+8) · 新加坡' },
  { value: 'Asia/Bangkok', label: 'Asia/Bangkok (UTC+7) · 泰国' },
  { value: 'Asia/Kolkata', label: 'Asia/Kolkata (UTC+5:30) · 印度' },
  { value: 'Asia/Dubai', label: 'Asia/Dubai (UTC+4) · 阿联酋' },
  { value: 'Europe/London', label: 'Europe/London (UTC+0 标准时) · 英国' },
  { value: 'Europe/Paris', label: 'Europe/Paris (UTC+1 标准时) · 法国' },
  { value: 'Europe/Berlin', label: 'Europe/Berlin (UTC+1 标准时) · 德国' },
  { value: 'Europe/Moscow', label: 'Europe/Moscow (UTC+3) · 俄罗斯' },
  { value: 'America/New_York', label: 'America/New_York (UTC−5 标准时) · 美国东部' },
  { value: 'America/Chicago', label: 'America/Chicago (UTC−6 标准时) · 美国中部' },
  { value: 'America/Los_Angeles', label: 'America/Los_Angeles (UTC−8 标准时) · 美国西部' },
  { value: 'America/Sao_Paulo', label: 'America/Sao_Paulo (UTC−3) · 巴西' },
  { value: 'Australia/Sydney', label: 'Australia/Sydney (UTC+10 标准时) · 澳大利亚' },
  { value: 'Pacific/Auckland', label: 'Pacific/Auckland (UTC+12 标准时) · 新西兰' },
  { value: 'UTC', label: 'UTC (UTC+0) · 协调世界时' },
]

/**
 * Returns the selectable zones for a stored value, appending one the list does
 * not offer so an existing timezone stays visible and selected instead of
 * silently reading as an empty control.
 */
export function timezonePickerOptions(current: string): TimezoneOption[] {
  const options = [...timezoneOptions]
  if (current && !options.some((option) => option.value === current)) {
    options.push({ value: current, label: current })
  }
  return options
}
