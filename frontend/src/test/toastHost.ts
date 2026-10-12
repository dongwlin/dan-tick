import { render } from '@testing-library/vue'
import UiToastHost from '@/components/UiToastHost.vue'

/**
 * Mounts the app-level toast host next to the component under test. `App.vue`
 * mounts it once for the whole application, so component tests that assert on
 * toast feedback need the same single viewport in the document.
 */
export function mountToastHost() {
  return render(UiToastHost)
}
