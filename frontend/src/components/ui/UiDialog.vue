<script setup lang="ts">
import { X } from 'lucide-vue-next'
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import UiIconButton from './UiIconButton.vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    /** Accessible Dialog title, also rendered as the visible heading. */
    title: string
    /** Optional supporting text, linked to the Dialog through `aria-describedby`. */
    description?: string
    /**
     * Blocks Escape and outside dismissal while a submission is running, so a
     * backdrop click cannot discard work the user already started (规范 7.2).
     * The caller states the reason, for example through a busy action label.
     */
    busy?: boolean
  }>(),
  { busy: false, description: '' },
)

const emit = defineEmits<{
  'update:open': [open: boolean]
  /** Mount autofocus, forwarded from DialogContent; prevent it to focus a field. */
  openAutoFocus: [event: Event]
}>()

function updateOpen(open: boolean) {
  if (!open && props.busy) return
  emit('update:open', open)
}

function preventClosingWhenBusy(event: Event) {
  if (props.busy) event.preventDefault()
}
</script>

<template>
  <DialogRoot :open="open" @update:open="updateOpen">
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-50 bg-[rgb(8_14_20/42%)]" />
      <DialogContent
        aria-modal="true"
        class="fixed top-1/2 left-1/2 z-50 max-h-[calc(100dvh_-_3rem)] w-104 max-w-[calc(100vw_-_3rem)] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl bg-surface-solid p-6 shadow-(--shadow-overlay)"
        @open-auto-focus="emit('openAutoFocus', $event)"
        @escape-key-down="preventClosingWhenBusy"
        @interact-outside="preventClosingWhenBusy"
      >
        <div class="mb-5 flex items-center justify-between gap-4">
          <DialogTitle class="text-lg font-semibold">{{ title }}</DialogTitle>
          <DialogClose as-child>
            <UiIconButton :label="`关闭${title}`" :disabled="busy">
              <X class="size-5" aria-hidden="true" />
            </UiIconButton>
          </DialogClose>
        </div>
        <DialogDescription :class="description ? 'mb-4 text-sm text-text-secondary' : 'sr-only'">
          {{ description }}
        </DialogDescription>
        <slot />
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
