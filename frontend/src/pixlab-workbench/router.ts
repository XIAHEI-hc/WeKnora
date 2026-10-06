import { createRouter, createWebHistory } from 'vue-router'

import InvalidWorkbenchRoute from './InvalidWorkbenchRoute.vue'
import NativeProjectApp from './NativeProjectApp.vue'
import { isAllowedProjectRoute } from './routePolicy'

const router = createRouter({
  history: createWebHistory('/weknora-workbench/'),
  routes: [
    {
      path: '/projects/:projectCode',
      component: NativeProjectApp,
      children: [
        {
          path: '',
          component: () => import('@/views/platform/index.vue'),
          props: { pixlabProject: true },
          children: [
            { path: '', redirect: { name: 'pixlabProjectKnowledge' } },
            {
              path: 'knowledge',
              name: 'pixlabProjectKnowledge',
              component: () => import('@/views/knowledge/KnowledgeBase.vue'),
              props: { pixlabProject: true },
            },
            {
              path: 'chat/new',
              name: 'pixlabProjectNewChat',
              component: () => import('@/views/creatChat/creatChat.vue'),
              props: { pixlabProject: true },
            },
            {
              path: 'chat/:chatid',
              name: 'pixlabProjectChat',
              component: () => import('@/views/chat/index.vue'),
              props: (route) => ({
                pixlabProject: true,
                embeddedMode: true,
                session_id: String(route.params.chatid || ''),
              }),
            },
          ],
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'pixlabWorkbenchInvalidRoute',
      component: InvalidWorkbenchRoute,
    },
  ],
})

router.beforeEach((to) => {
  if (to.name === 'pixlabWorkbenchInvalidRoute') return true
  const projectCode = String(to.params.projectCode || '')
  if (!projectCode || !isAllowedProjectRoute(to.path, projectCode)) {
    return { name: 'pixlabWorkbenchInvalidRoute' }
  }
  return true
})

export default router
