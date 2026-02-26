import { createRouter, createWebHashHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import Login from '../views/Login.vue'
import Layout from '../components/Layout.vue'
import Services from '../views/Services.vue'
import Hosts from '../views/Hosts.vue'
import K8S from '../views/K8S.vue'
import K8SDetail from '../views/K8SDetail.vue'
import Logs from '../views/Logs.vue'
import Devices from '../views/Devices.vue'

const routes = [
  {
    path: '/',
    redirect: '/services'
  },
  {
    path: '/login',
    name: 'Login',
    component: Login
  },
  {
    path: '/',
    component: Layout,
    meta: { requiresAuth: true },
    children: [
      {
        path: 'services',
        name: 'Services',
        component: Services
      },
      {
        path: 'hosts',
        name: 'Hosts',
        component: Hosts
      },
      {
        path: 'k8s',
        name: 'K8S',
        component: K8S
      },
      {
        path: 'k8s/:domain',
        name: 'K8SDetail',
        component: K8SDetail
      },
      {
        path: 'logs',
        name: 'Logs',
        component: Logs
      },
      {
        path: 'devices',
        name: 'Devices',
        component: Devices
      }
    ]
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
