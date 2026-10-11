<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import HistoryDetail from '@/features/history/HistoryDetail.vue'

const route = useRoute()

const executionID = computed(() => {
  const value = route.params.id
  return typeof value === 'string' && /^[1-9]\d*$/.test(value) ? Number(value) : 0
})
</script>

<template>
  <HistoryDetail :execution-id="executionID" />
</template>
