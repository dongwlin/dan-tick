<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import TaskEditorForm from '@/features/configuration/TaskEditorForm.vue'
import { bindingError, useTaskDetail } from '@/features/configuration/queries'

const route = useRoute()
const taskID = computed(() => Number(route.params.id))
const task = useTaskDetail(taskID)
</script>

<template>
  <h1 v-if="!Number.isSafeInteger(taskID) || taskID <= 0">编辑任务</h1>
  <p v-if="!Number.isSafeInteger(taskID) || taskID <= 0" role="alert" class="text-soft-danger-fg">
    任务编号无效
  </p>
  <div v-else-if="task.isPending.value" role="status">正在加载任务…</div>
  <div v-else-if="task.isError.value" role="alert">
    <h1>编辑任务</h1>
    <p class="text-soft-danger-fg">{{ bindingError(task.error.value) }}</p>
    <button type="button" class="underline" @click="task.refetch()">重试读取</button>
  </div>
  <TaskEditorForm v-else-if="task.data.value" :key="taskID" :task="task.data.value" />
</template>
