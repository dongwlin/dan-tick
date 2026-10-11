<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <section>
    <h1>页面不存在</h1>
    <p>请求的地址没有对应的页面，可能已被移动或删除。</p>
    <RouterLink to="/">返回总览</RouterLink>
  </section>
</template>
