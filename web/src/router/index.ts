import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { triggerDailyLogin } from '@/api/daily'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/HomeView.vue'),
  },
  {
    path: '/video/:id',
    name: 'video',
    component: () => import('@/views/VideoDetailView.vue'),
  },
  {
    path: '/search',
    name: 'search',
    component: () => import('@/views/SearchView.vue'),
  },
  {
    path: '/user/:id',
    name: 'user',
    component: () => import('@/views/UserHomeView.vue'),
  },
  {
    path: '/space',
    component: () => import('@/views/SpaceView.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/space/favorites' },
      {
        path: 'favorites',
        name: 'space-favorites',
        component: () => import('@/views/space/FavoritesView.vue'),
      },
      {
        path: 'history',
        name: 'space-history',
        component: () => import('@/views/space/HistoryView.vue'),
      },
      {
        path: 'notifications',
        name: 'space-notifications',
        component: () => import('@/views/space/NotificationsView.vue'),
      },
      {
        path: 'coin-ledger',
        name: 'space-coin-ledger',
        component: () => import('@/views/space/CoinLedgerView.vue'),
      },
      {
        path: 'daily',
        name: 'space-daily',
        component: () => import('@/views/space/DailyTaskView.vue'),
      },
      {
        path: 'profile',
        name: 'space-profile',
        component: () => import('@/views/space/ProfileEditView.vue'),
      },
      {
        path: 'followers',
        name: 'space-followers',
        component: () => import('@/views/space/FollowListView.vue'),
        props: { mode: 'followers' },
      },
      {
        path: 'following',
        name: 'space-following',
        component: () => import('@/views/space/FollowListView.vue'),
        props: { mode: 'following' },
      },
    ],
  },
  {
    path: '/forgot-password',
    name: 'forgot-password',
    component: () => import('@/views/ForgotPasswordView.vue'),
  },
  {
    path: '/dynamic',
    name: 'dynamic',
    component: () => import('@/views/DynamicView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/article/:id',
    name: 'article',
    component: () => import('@/views/ArticleDetailView.vue'),
  },
  {
    path: '/upload',
    name: 'upload',
    component: () => import('@/views/UploadView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

// 每日访问奖励：已登录用户每天首次访问网站触发 +10 经验（后端幂等，一天只加一次）
// 使用 sessionStorage 标记本次会话已触发，避免路由切换时重复请求
const DAILY_TRIGGER_KEY = '__daily_login_triggered__'
let dailyTriggeredThisSession = false
try {
  dailyTriggeredThisSession = sessionStorage.getItem(DAILY_TRIGGER_KEY) === '1'
} catch {
  // ignore
}

// 路由守卫
router.beforeEach((to, _from, next) => {
  const userStore = useUserStore()
  if (to.meta.requiresAuth && !userStore.isLogin) {
    next({ path: '/login', query: { redirect: to.fullPath } })
  } else {
    next()
  }
  // 已登录且本次会话未触发过：触发每日访问奖励（fire-and-forget）
  // 后端按上海时区日期幂等，一天只加一次经验
  if (userStore.isLogin && !dailyTriggeredThisSession) {
    dailyTriggeredThisSession = true
    try {
      sessionStorage.setItem(DAILY_TRIGGER_KEY, '1')
    } catch {
      // ignore
    }
    triggerDailyLogin().catch(() => {})
  }
})

router.afterEach((to) => {
  const titles: Record<string, string> = {
    home: '首页',
    video: '视频',
    search: '搜索',
    user: '用户主页',
    'space-favorites': '我的收藏',
    'space-history': '历史记录',
    'space-notifications': '消息',
    'space-coin-ledger': '硬币流水',
    'space-daily': '每日任务',
    'space-profile': '个人资料',
    dynamic: '动态',
    article: '专栏',
    upload: '投稿',
    login: '登录',
  }
  document.title = `${titles[to.name as string] || ''} - 仿B站`
})

export default router
