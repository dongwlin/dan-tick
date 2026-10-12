<script setup lang="ts">
import { computed, watch } from 'vue'
import {
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { buttonClass } from './buttonStyles'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description: string
    confirmLabel?: string
    cancelLabel?: string
    variant?: 'default' | 'danger'
    /**
     * Renders a regular Dialog that can be dismissed with Escape or an outside
     * click. Dismissal is reported as `cancel`. Keep it `false` for
     * irreversible actions, which need Alert Dialog semantics (规范 9.4).
     */
    dismissible?: boolean
  }>(),
  {
    confirmLabel: '确认',
    cancelLabel: '取消',
    variant: 'danger',
    dismissible: false,
  },
)

const emit = defineEmits<{
  'update:open': [open: boolean]
  confirm: []
  cancel: []
}>()

const rootComponent = computed(() => (props.dismissible ? DialogRoot : AlertDialogRoot))
const portalComponent = computed(() => (props.dismissible ? DialogPortal : AlertDialogPortal))
const overlayComponent = computed(() => (props.dismissible ? DialogOverlay : AlertDialogOverlay))
const contentComponent = computed(() => (props.dismissible ? DialogContent : AlertDialogContent))
const titleComponent = computed(() => (props.dismissible ? DialogTitle : AlertDialogTitle))
const descriptionComponent = computed(() =>
  props.dismissible ? DialogDescription : AlertDialogDescription,
)

const confirmClass = computed(() => buttonClass(props.variant === 'danger' ? 'danger' : 'primary'))
const cancelClass = buttonClass('secondary')

// Distinguishes an explicit button choice from Escape/outside dismissal so
// dismissal can be reported as `cancel` exactly once.
let decided = false
watch(
  () => props.open,
  (open) => {
    if (open) decided = false
  },
)

function close(decision: 'confirm' | 'cancel') {
  decided = true
  if (decision === 'confirm') emit('confirm')
  else emit('cancel')
  emit('update:open', false)
}

function updateOpen(open: boolean) {
  if (!open && !decided) {
    decided = true
    emit('cancel')
  }
  emit('update:open', open)
}

function onEscapeKeyDown(event: Event) {
  if (!props.dismissible) event.preventDefault()
}
</script>

<template>
  <component :is="rootComponent" :open="open" @update:open="updateOpen">
    <component :is="portalComponent">
      <component :is="overlayComponent" class="fixed inset-0 z-50 bg-[rgb(8_14_20/42%)]" />
      <component
        :is="contentComponent"
        aria-modal="true"
        class="fixed top-1/2 left-1/2 z-50 max-h-[calc(100dvh_-_3rem)] w-104 max-w-[calc(100vw_-_3rem)] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl bg-surface-solid p-6 shadow-(--shadow-overlay)"
        @escape-key-down="onEscapeKeyDown"
      >
        <component :is="titleComponent" class="text-lg font-semibold">{{ title }}</component>
        <component :is="descriptionComponent" class="mt-2 text-sm text-text-secondary">
          {{ description }}
        </component>
        <!-- Cancel comes first so both Dialog and Alert Dialog open with focus
             on the safe action, as required by 规范 7.2. -->
        <div class="mt-5 flex justify-end gap-2">
          <button type="button" :class="cancelClass" @click="close('cancel')">
            {{ cancelLabel }}
          </button>
          <button type="button" :class="confirmClass" @click="close('confirm')">
            {{ confirmLabel }}
          </button>
        </div>
      </component>
    </component>
  </component>
</template>
