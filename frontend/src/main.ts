import { createApp } from 'vue'
import App from './app/App.vue'
import router from './app/router'
import { initTheme } from './app/theme'
import './style.css'

initTheme()
createApp(App).use(router).mount('#app')
