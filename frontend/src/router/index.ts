import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('@/components/layout/AppLayout.vue'),
      children: [
        {
          path: '',
          name: 'files',
          component: () => import('@/views/FilesView.vue'),
        },
        {
          path: 'admin',
          name: 'admin',
          component: () => import('@/views/AdminView.vue'),
          meta: { admin: true },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/SettingsView.vue'),
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/',
    },
  ],
})

router.afterEach((to) => {
  if (to.name === 'files') {
    const path = to.query.path as string | undefined
    const folder = path ? path.split('/').pop()! : 'Home'
    document.title = `${folder} - files`
  } else {
    const names: Record<string, string> = {
      login: 'Login',
      admin: 'Admin',
      settings: 'Settings',
    }
    const label = names[to.name as string]
    document.title = label ? `${label} - files` : 'files'
  }
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  await auth.restoreSession()

  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  if (to.meta.admin && !auth.isAdmin) {
    return { name: 'files' }
  }

  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'files' }
  }
})

// After the user is logged out (logout, or the session ended): opens the login
// page, then forgets the user's state. In that order: views still showing the
// files would react to a cleared listing by rewriting the URL, which would
// overtake this navigation. With redirect, logging in again returns there.
export async function showLoginAfterLogout(redirect?: string) {
  await router.push({ name: 'login', query: redirect ? { redirect } : {} })
  useAuthStore().clearUserState()
}

export default router
