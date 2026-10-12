<script setup lang="ts">
import { computed, useId } from 'vue'
import { labelClass } from './uiStyles'

const props = defineProps<{
  /** Persistent visible label, rendered as a real `label` or `legend`. */
  label: string
  /** Id of the control the label points at. Generated when omitted. */
  id?: string
  /** Field error, shown next to the label and linked through `aria-describedby`. */
  error?: string
  /** Persistent hint shown under the control and linked through `aria-describedby`. */
  description?: string
  /** Render a `fieldset`/`legend` group instead of a `label`/control pair. */
  group?: boolean
}>()

const generatedId = useId()
const controlId = computed(() => props.id ?? `ui-field-${generatedId}`)
const errorId = computed(() => `${controlId.value}-error`)
const descriptionId = computed(() => `${controlId.value}-description`)
// Omit `aria-invalid` while the field is valid, so assistive technology reads a
// missing state instead of an explicit "false".
const ariaInvalid = computed(() => (props.error ? (true as const) : undefined))
const describedBy = computed(() =>
  props.description ? `${errorId.value} ${descriptionId.value}` : errorId.value,
)
</script>

<template>
  <component :is="group ? 'fieldset' : 'div'" :class="group ? undefined : labelClass">
    <legend v-if="group" class="mb-2 w-full text-sm text-text-secondary">
      <span class="flex min-w-0 items-center justify-between gap-2">
        <span class="shrink-0">{{ label }}</span>
        <span
          :id="errorId"
          :title="error || undefined"
          class="min-w-0 truncate text-right text-xs leading-5 text-soft-danger-fg"
          >{{ error }}</span
        >
      </span>
    </legend>
    <span v-else class="flex min-w-0 items-center justify-between gap-2">
      <label :for="controlId" class="shrink-0">{{ label }}</label>
      <span
        :id="errorId"
        :title="error || undefined"
        class="min-w-0 truncate text-right text-xs leading-5 text-soft-danger-fg"
        >{{ error }}</span
      >
    </span>
    <slot :id="controlId" :aria-invalid="ariaInvalid" :described-by="describedBy" />
    <span
      v-if="description"
      :id="descriptionId"
      class="block text-xs leading-5 text-text-tertiary"
      >{{ description }}</span
    >
  </component>
</template>
