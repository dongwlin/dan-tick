<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <div class="app-frame">
    <!-- Temporary navigation so every route is reachable before the real shell
         exists; F3 replaces it with the sidebar from the approved prototype. -->
    <nav class="temp-nav" aria-label="主导航">
      <RouterLink to="/">总览</RouterLink>
      <RouterLink to="/tasks">弹幕任务</RouterLink>
      <RouterLink to="/rooms">直播间</RouterLink>
      <RouterLink to="/history">执行记录</RouterLink>
      <RouterLink to="/settings">设置</RouterLink>
    </nav>
    <main class="app-content">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.app-frame {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.temp-nav {
  display: flex;
  gap: 20px;
  padding: 16px 24px;
  border-bottom: 1px solid lightgray;
}

.app-content {
  flex: 1;
  padding: 24px;
}
</style>
