import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router, { showLoginAfterLogout } from './router'
import { useAuthStore } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { onSessionEnded } from '@/utils/api'
import './style.css'

// When a new build deploys while users are on the page, lazy-loaded chunks from
// the previous build will 404. Reload so they get the fresh index.html + chunks.
window.addEventListener('vite:preloadError', () => window.location.reload())

// Older versions kept a login token in localStorage; logins are cookie sessions now.
try { localStorage.removeItem('token') } catch { /* storage unavailable */ }

const app = createApp(App)

app.use(createPinia())
app.use(router)

// When the session ends mid-use (expired, ended elsewhere), go to the login
// page, and back to the same place after logging in.
onSessionEnded(() => {
  const redirect = router.currentRoute.value.fullPath
  if (!useAuthStore().sessionEnded()) return
  useToast().show('Your session has ended. Please log in again.', 'info')
  showLoginAfterLogout(redirect)
})

app.mount('#app')
