<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const executionID = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? id : ''
})
</script>

<template>
  <section>
    <h1>执行详情</h1>
    <p>执行详情页面尚未接入，将在 F6 阶段按定稿原型迁移。</p>
    <p v-if="executionID">当前路由执行 ID：{{ executionID }}</p>
  </section>
</template>
