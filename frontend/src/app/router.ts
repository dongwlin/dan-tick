import { createRouter, createWebHashHistory } from 'vue-router'

// Hash history keeps routes working inside the Wails webview, where the app is
// served without a server that could fall back to index.html on deep links.
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'overview',
      component: () => import('@/pages/OverviewPage.vue'),
    },
    {
      path: '/tasks',
      name: 'tasks',
      component: () => import('@/pages/TasksPage.vue'),
    },
    {
      path: '/tasks/new',
      name: 'task-create',
      component: () => import('@/pages/TaskCreatePage.vue'),
    },
    {
      path: '/tasks/:id/edit',
      name: 'task-edit',
      component: () => import('@/pages/TaskEditPage.vue'),
    },
    {
      path: '/rooms',
      name: 'rooms',
      component: () => import('@/pages/RoomsPage.vue'),
    },
    {
      path: '/history',
      name: 'history',
      component: () => import('@/pages/HistoryPage.vue'),
    },
    {
      path: '/history/:id',
      name: 'history-detail',
      component: () => import('@/pages/HistoryDetailPage.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/pages/SettingsPage.vue'),
    },
    {
      // Navigation fallback so unknown addresses render an accessible page.
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/pages/NotFoundPage.vue'),
    },
  ],
})

export default router
