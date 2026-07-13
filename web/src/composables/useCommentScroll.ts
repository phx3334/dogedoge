import { watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'

// 当路由带 ?comment_id= 时，等待评论渲染后滚动并高亮对应评论。
// 评论列表为异步加载，故以轮询重试方式定位元素。
export function useCommentScroll() {
  const route = useRoute()
  let timer: ReturnType<typeof setTimeout> | null = null

  function scrollToComment(commentId: string): boolean {
    const el = document.querySelector(
      `[data-comment-id="${commentId}"]`,
    ) as HTMLElement | null
    if (!el) return false
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el.classList.add('ring-2', 'ring-primary', 'rounded')
    setTimeout(() => el.classList.remove('ring-2', 'ring-primary', 'rounded'), 2500)
    return true
  }

  function tryScroll() {
    const cid = route.query.comment_id as string
    if (!cid) return
    let tries = 0
    const attempt = () => {
      if (scrollToComment(cid)) return
      if (tries++ < 30) {
        timer = setTimeout(attempt, 200)
      }
    }
    attempt()
  }

  onMounted(() => {
    if (route.query.comment_id) {
      setTimeout(tryScroll, 300)
    }
  })

  watch(
    () => route.query.comment_id,
    (cid) => {
      if (cid) setTimeout(tryScroll, 300)
    },
  )

  onUnmounted(() => {
    if (timer) clearTimeout(timer)
  })
}
