<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import TaskEditorForm from '@/features/configuration/TaskEditorForm.vue'
import { useTaskDetail } from '@/features/configuration/queries'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiAsyncSection from '@/components/ui/UiAsyncSection.vue'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'

const route = useRoute()
const taskID = computed(() => Number(route.params.id))
const invalid = computed(() => !Number.isSafeInteger(taskID.value) || taskID.value <= 0)
const task = useTaskDetail(taskID)
</script>

<template>
  <UiPageHeader v-if="!task.data.value" title="编辑任务" />
  <UiAlert v-if="invalid" tone="danger">任务编号无效</UiAlert>
  <UiAsyncSection
    v-else
    :pending="task.isPending.value"
    :error="task.error.value"
    loading-text="正在加载任务…"
    @retry="task.refetch()"
  >
    <TaskEditorForm v-if="task.data.value" :key="taskID" :task="task.data.value" />
  </UiAsyncSection>
</template>
