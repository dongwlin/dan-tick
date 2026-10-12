import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// DOM tests do not need the Wails desktop runtime or generated bindings.
export default defineConfig({
  resolve: { tsconfigPaths: true },
  plugins: [vue()],
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.test.ts'],
    setupFiles: ['./src/test/setup.ts'],
    // Keep the test timeout above the Testing Library async wait window.
    testTimeout: 15000,
  },
})
