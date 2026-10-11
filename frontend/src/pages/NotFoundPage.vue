<script setup lang="ts">
import { ArrowLeft } from 'lucide-vue-next'
import UiButton from '@/components/UiButton.vue'
</script>

<template>
  <section>
    <h1>页面不存在</h1>
    <p>请求的地址没有对应的页面，可能已被移动或删除。</p>
    <UiButton to="/" class="mt-4">
      <ArrowLeft class="size-4" aria-hidden="true" />
      返回总览
    </UiButton>
  </section>
</template>
