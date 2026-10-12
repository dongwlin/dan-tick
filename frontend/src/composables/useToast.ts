import { readonly, ref } from 'vue'

/** Feedback tone; it selects the toast icon, border and live region role. */
export type ToastTone = 'success' | 'error' | 'warning' | 'info'

export interface ToastMessage {
  id: number
  message: string
  tone: ToastTone
}

// The app mounts exactly one toast viewport (`UiToastHost`), so feedback is
// shared state instead of per-panel state. A new message replaces the previous
// one: stacking duplicates repeats announcements for screen readers and would
// overlap inside the single fixed viewport (可访问性规范 7.5).
const current = ref<ToastMessage | null>(null)
let nextToastId = 0

export function notify(message: string, tone: ToastTone = 'success'): void {
  current.value = { id: ++nextToastId, message, tone }
}

/** Dismisses the toast with `id`; without an id it clears whatever is shown. */
export function dismissToast(id?: number): void {
  if (id === undefined || current.value?.id === id) current.value = null
}

/** Clears the shared slot; tests reset it between cases. */
export function clearToasts(): void {
  current.value = null
}

/** Shared toast slot: one readonly message plus the actions that change it. */
export function useToast() {
  return { toast: readonly(current), notify, dismiss: dismissToast }
}
