import { computed, readonly, shallowRef, watch } from 'vue'

export type ThemePreference = 'dark' | 'light' | 'system'

const storageKey = 'dan-tick-theme'

function readPreference(): ThemePreference {
  try {
    const value = localStorage.getItem(storageKey)
    if (value === 'dark' || value === 'light' || value === 'system') return value
  } catch {
    // Storage can be unavailable in restricted WebViews; the UI still works.
  }
  return 'dark'
}

const preference = shallowRef<ThemePreference>(readPreference())
const systemDark = shallowRef(false)
const activeTheme = computed(() =>
  preference.value === 'system' ? (systemDark.value ? 'dark' : 'light') : preference.value,
)

export function initTheme(): void {
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  systemDark.value = media.matches
  media.addEventListener('change', (event) => {
    systemDark.value = event.matches
  })

  watch(
    activeTheme,
    (theme) => {
      document.documentElement.dataset.theme = theme
    },
    { immediate: true, flush: 'sync' },
  )
}

function setPreference(next: ThemePreference): void {
  preference.value = next
  try {
    localStorage.setItem(storageKey, next)
  } catch {
    // Keep the selected theme for this session even if persistence is denied.
  }
}

export function useTheme() {
  return { preference: readonly(preference), activeTheme, setPreference }
}
