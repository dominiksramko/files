import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import './style.css'

// When a new build deploys while users are on the page, lazy-loaded chunks from
// the previous build will 404. Reload so they get the fresh index.html + chunks.
window.addEventListener('vite:preloadError', () => window.location.reload())

// Older versions kept a login token in localStorage; logins are cookie sessions now.
try { localStorage.removeItem('token') } catch { /* storage unavailable */ }

const app = createApp(App)

app.use(createPinia())
app.use(router)

app.mount('#app')
