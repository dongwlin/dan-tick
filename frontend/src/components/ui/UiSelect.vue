<script setup lang="ts" generic="T extends string | number">
import { computed } from 'vue'
import { Check, ChevronDown, ChevronUp } from 'lucide-vue-next'
import {
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectScrollDownButton,
  SelectScrollUpButton,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from 'reka-ui'

interface UiSelectOption {
  value: T
  label: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    /** Rows in display order. The current value is matched by identity, not by index. */
    options: readonly UiSelectOption[]
    /** Current value; a value that matches no option reads as the placeholder. */
    modelValue?: T
    /** Shown while no option matches the current value. */
    placeholder?: string
    disabled?: boolean
    /** Marks the field as required for assistive technology. */
    required?: boolean
  }>(),
  { modelValue: undefined, placeholder: '', disabled: false, required: false },
)

const emit = defineEmits<{
  'update:modelValue': [value: T]
  /** Focus left the trigger, so a form can mark the field as touched. */
  blur: [event: FocusEvent]
}>()

// The visible box style (border, surface, height, width) stays with the caller
// through `class`, exactly like `fieldClass` on the native controls before.
defineOptions({ inheritAttrs: false })

// The trigger text comes from `options` instead of the primitive's own tracking:
// that tracking only learns an option's text once its item has been mounted, so
// a stored value would read as the placeholder until the list was opened once.
const displayLabel = computed(
  () =>
    props.options.find((option) => option.value === props.modelValue)?.label ?? props.placeholder,
)
</script>

<template>
  <SelectRoot
    :model-value="props.modelValue"
    :disabled="props.disabled"
    :required="props.required"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <SelectTrigger
      v-bind="$attrs"
      class="flex cursor-pointer items-center justify-between gap-2 text-left"
      @blur="emit('blur', $event)"
    >
      <SelectValue
        class="min-w-0 truncate"
        :class="displayLabel === props.placeholder ? 'text-text-tertiary' : undefined"
      >
        {{ displayLabel }}
      </SelectValue>
      <SelectIcon as-child>
        <ChevronDown class="size-4 shrink-0 text-text-tertiary" aria-hidden="true" />
      </SelectIcon>
    </SelectTrigger>
    <SelectPortal>
      <SelectContent
        position="popper"
        side="bottom"
        align="start"
        :side-offset="4"
        :body-lock="false"
        class="z-50 max-h-(--reka-select-content-available-height) max-w-(--reka-select-content-available-width) min-w-(--reka-select-trigger-width) overflow-hidden rounded-[10px] border border-border-hairline bg-surface-solid text-sm text-text-primary shadow-(--shadow-overlay)"
      >
        <SelectScrollUpButton
          class="flex h-6 shrink-0 items-center justify-center text-text-tertiary"
        >
          <ChevronUp class="size-4" aria-hidden="true" />
        </SelectScrollUpButton>
        <SelectViewport class="p-1">
          <SelectItem
            v-for="option in props.options"
            :key="option.value"
            :value="option.value"
            :disabled="option.disabled"
            class="flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-text-primary select-none data-highlighted:bg-surface-solid-hover data-disabled:cursor-not-allowed data-disabled:opacity-60"
          >
            <SelectItemText class="flex-1">{{ option.label }}</SelectItemText>
            <SelectItemIndicator class="shrink-0 text-accent-text">
              <Check class="size-3.5" aria-hidden="true" />
            </SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
        <SelectScrollDownButton
          class="flex h-6 shrink-0 items-center justify-center text-text-tertiary"
        >
          <ChevronDown class="size-4" aria-hidden="true" />
        </SelectScrollDownButton>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>
