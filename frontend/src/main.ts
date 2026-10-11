import { createApp } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import App from './app/App.vue'
import { createAppQueryClient } from './app/queryClient'
import router from './app/router'
import { initTheme } from './app/theme'
import './style.css'

initTheme()
createApp(App)
  .use(router)
  .use(VueQueryPlugin, { queryClient: createAppQueryClient() })
  .mount('#app')
