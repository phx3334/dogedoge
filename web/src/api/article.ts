import request from './request'
import type { ArticleDetailResp } from '@/types'

// 文章详情
export function getArticleDetail(article_id: number) {
  return request.get<any, ArticleDetailResp>('/article/detail', { params: { article_id } })
}

// 保存文章草稿
// 后端 ArticleDraftReq 字段：title / body_md / cover_url / tags / images
// 后端返回 { article_id: number }
export function saveArticleDraft(data: {
  title: string
  body_md: string
  cover_url?: string
  tags?: string[]
  images?: string[]
}) {
  return request.post<any, { article_id: number }>('/article/draft', data)
}

// 发布文章
// 后端 ArticlePublishReq 字段：article_id（仅需 article_id 即可发布已有草稿）
export function publishArticle(article_id: number) {
  return request.post<any, void>('/article/publish', { article_id })
}
