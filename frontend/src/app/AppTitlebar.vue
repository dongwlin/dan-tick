<script setup lang="ts">
import { Minus, Power, Square, X } from 'lucide-vue-next'
import { shallowRef } from 'vue'
import titlebarWordmark from '@/assets/branding/titlebar-wordmark.png'

const error = shallowRef('')

async function controlWindow(action: 'minimise' | 'maximise' | 'close' | 'quit') {
  error.value = ''
  // The Wails bootstrap sets the platform environment before mounting the UI.
  // Avoid importing the desktop runtime (and its browser transport) in web previews.
  const os = (window as Window & { _wails?: { environment?: { OS?: string } } })._wails?.environment
    ?.OS
  if (os !== 'windows' && os !== 'linux') {
    error.value = '浏览器预览无法控制窗口，请在 Wails 桌面应用中使用。'
    return
  }

  try {
    if (action === 'quit') {
      // Native confirmation also handles OS close shortcuts in quit mode.
      const { Quit } =
        await import('@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice')
      await Quit()
      return
    }
    const { Window } = await import('@wailsio/runtime')
    if (action === 'minimise') await Window.Minimise()
    else if (action === 'maximise') await Window.ToggleMaximise()
    else await Window.Close()
  } catch {
    error.value = '窗口操作失败，请重试。'
  }
}
</script>

<template>
  <header
    class="window-titlebar relative z-20 flex h-[42px] shrink-0 items-stretch justify-between border-b border-border-hairline bg-bg-titlebar select-none"
    aria-label="窗口标题栏"
  >
    <img
      :src="titlebarWordmark"
      alt="dan-tick"
      width="105"
      height="35"
      class="pointer-events-none ml-5 h-[35px] w-auto self-center object-contain opacity-75"
      draggable="false"
    />
    <div class="window-controls flex items-stretch" role="group" aria-label="窗口操作">
      <button
        type="button"
        class="window-control"
        aria-label="退出应用"
        title="退出应用（停止后台调度）"
        @click="controlWindow('quit')"
      >
        <Power class="size-3.5" aria-hidden="true" />
      </button>
      <button
        type="button"
        class="window-control"
        aria-label="最小化窗口"
        title="最小化"
        @click="controlWindow('minimise')"
      >
        <Minus class="size-3.5" aria-hidden="true" />
      </button>
      <button
        type="button"
        class="window-control"
        aria-label="最大化或还原窗口"
        title="最大化或还原"
        @click="controlWindow('maximise')"
      >
        <Square class="size-3.5" aria-hidden="true" />
      </button>
      <button
        type="button"
        class="window-control hover:!bg-[#bc3f38] hover:!text-white active:!bg-[#bc3f38]"
        aria-label="关闭窗口"
        title="关闭窗口（默认隐藏到后台，再次启动可恢复）"
        @click="controlWindow('close')"
      >
        <X class="size-3.5" aria-hidden="true" />
      </button>
    </div>
    <p
      v-if="error"
      role="alert"
      class="absolute top-[48px] right-4 z-30 max-w-sm rounded-[10px] border border-soft-warning-border bg-soft-warning-bg px-3 py-2 text-xs text-soft-warning-fg shadow-(--shadow-overlay) [--wails-draggable:no-drag]"
    >
      {{ error }}
    </p>
  </header>
</template>

<style scoped>
@reference '../style.css';

.window-titlebar {
  --wails-draggable: drag;
}

.window-controls,
.window-control {
  --wails-draggable: no-drag;
}

.window-control {
  @apply grid h-[41px] w-[47px] shrink-0 place-items-center border-0 bg-transparent text-text-secondary hover:bg-surface-solid-hover hover:text-text-primary active:bg-surface-muted;
}
</style>
