<script setup lang="ts">
import { computed } from 'vue'
import { bindingError } from '@/lib/bindingError'
import UiButton from './UiButton.vue'

const props = withDefaults(
  defineProps<{
    pending: boolean
    /** Binding error from a failed read; null/undefined means there is no error. */
    error?: unknown
    loadingText?: string
    /** Overrides the safe Binding error text when the surface needs custom wording. */
    errorText?: string
    retryLabel?: string
    /** Compact vertical rhythm for error blocks embedded in a padded panel. */
    dense?: boolean
  }>(),
  {
    error: undefined,
    loadingText: '正在加载…',
    errorText: undefined,
    retryLabel: '重试读取',
    dense: false,
  },
)

const emit = defineEmits<{ retry: [] }>()

const statePadding = computed(() => (props.dense ? 'py-4' : 'p-6'))
const hasError = computed(
  () =>
    props.error !== undefined &&
    props.error !== null &&
    props.error !== false &&
    props.error !== '',
)
const message = computed(() =>
  props.errorText !== undefined ? props.errorText : bindingError(props.error),
)
</script>

<template>
  <div v-if="pending" role="status" :class="['text-sm text-text-secondary', statePadding]">
    {{ loadingText }}
  </div>
  <div v-else-if="hasError" role="alert" :class="statePadding">
    <p class="text-soft-danger-fg">{{ message }}</p>
    <UiButton v-if="retryLabel" variant="secondary" class="mt-3" @click="emit('retry')">
      {{ retryLabel }}
    </UiButton>
  </div>
  <slot v-else />
</template>
