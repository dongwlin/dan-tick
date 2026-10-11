<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const taskID = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? id : ''
})
</script>

<template>
  <section>
    <h1>编辑任务</h1>
    <p>任务编辑页面尚未接入，将在 F4 阶段按定稿原型迁移。</p>
    <p v-if="taskID">当前路由任务 ID：{{ taskID }}</p>
  </section>
</template>
