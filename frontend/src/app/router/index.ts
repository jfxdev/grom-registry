import { useSessionStore } from '@/modules/auth/store/session'
import { ROUTES } from '@/shared/constants'
import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: ROUTES.signIn,
      name: 'signin',
      component: () => import('@/modules/auth/pages/SignInPage.vue'),
      meta: { public: true },
    },
    {
      path: ROUTES.resetPassword,
      name: 'reset-password',
      component: () => import('@/modules/auth/pages/ResetPasswordPage.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      redirect: ROUTES.projects,
    },
    {
      path: ROUTES.projects,
      name: 'projects',
      component: () => import('@/modules/projects/pages/ProjectsPage.vue'),
    },
    {
      path: ROUTES.profile,
      name: 'profile',
      component: () => import('@/modules/users/pages/UserProfilePage.vue'),
    },
    {
      path: `${ROUTES.projects}/:project`,
      name: 'project-detail',
      component: () => import('@/modules/projects/pages/ProjectPage.vue'),
    },
    {
      path: ROUTES.users,
      name: 'users',
      component: () => import('@/modules/users/pages/UsersPage.vue'),
      meta: { requiresAdmin: true },
    },
    {
      path: ROUTES.serviceAccounts,
      name: 'service-accounts',
      component: () => import('@/modules/service-accounts/pages/ServiceAccountsPage.vue'),
    },
    {
      path: ROUTES.integrations,
      name: 'integrations',
      component: () => import('@/modules/integrations/pages/IntegrationsPage.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: ROUTES.projects,
    },
  ],
})

router.beforeEach(async (to) => {
  const session = useSessionStore()
  if (!session.checked) {
    await session.restore()
  }
  if (!to.meta.public && !session.user) {
    return { name: 'signin', query: { redirect: to.fullPath } }
  }
  if (to.name === 'reset-password' && session.user) {
    return { name: 'profile' }
  }
  if (to.meta.requiresAdmin && !session.user?.systemAdmin) {
    return { name: 'projects' }
  }
  if (to.name === 'signin' && session.user) {
    return { name: 'projects' }
  }
})
