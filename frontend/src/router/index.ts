import { createRouter, createWebHashHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import Login from '../views/Login.vue'
import Services from '../views/Services.vue'
import Devices from '../views/Devices.vue'
import Logs from '../views/Logs.vue'

const routes = [
  {
    path: '/',
    redirect: '/login'
  },
  {
    path: '/login',
    name: 'Login',
    component: Login
  },
  {
    path: '/services',
    name: 'Services',
    component: Services,
    meta: { requiresAuth: true }
  },
  {
    path: '/devices',
    name: 'Devices',
    component: Devices,
    meta: { requiresAuth: true }
  },
  {
    path: '/logs',
    name: 'Logs',
    component: Logs
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

// 路由守卫
router.beforeEach((to, from, next) => {
  const authStore = useAuthStore()
  
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    next('/login')
  } else if (to.path === '/login' && authStore.isAuthenticated) {
    next('/services')
  } else {
    next()
  }
})

export default router
