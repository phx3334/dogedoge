// 统一响应类型
export interface ApiResponse<T = any> {
  code: 3 | 4 // 3=成功, 4=失败
  data: T
  msg: string
}

// 分页响应
export interface PaginatedResp<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

// 用户
export interface Account {
  id: string
  username: string
  email: string
  avatar_url: string
  signature: string
  fans_count: number
  follow_count: number
  experience: number
  level: number
  coin_balance_tenths: number
  // 编辑资料页使用（后端 /user/info 返回的扩展字段）
  gender?: 'male' | 'female' | 'secret' | string
  birthday?: string
  address?: string
}

export interface LoginResp {
  access_token: string
  access_token_expire: number
  account: Account
}

export interface UserLevelResp {
  level: number
  experience: number
  current_level_exp: number
  next_level_exp: number
  max_level_exp: number
  is_max_level: boolean
}

// 视频
export interface HomeVideoInfo {
  id: number
  up_name: string
  up_avatar: string
  title: string
  cover_url: string
  play_count: number
  comment_count: number
  duration: number
  created_at: string
  fav_count: number
}

export interface AuthorInfo {
  id: string
  username: string
  avatar_url: string
  signature: string
  fans_count: number
}

export interface InteractionStatusResp {
  is_liked: boolean
  is_favorited: boolean
  coin_count: number
  is_followed: boolean
}

export interface VideoDetailResp {
  id: number
  title: string
  description: string
  play_url: string
  cover_url: string
  duration: number
  zone: string
  play_count: number
  likes_count: number
  comment_count: number
  fav_count: number
  coin_count: number
  danmaku_count: number
  comments_closed: boolean
  danmaku_closed: boolean
  created_at: string
  author: AuthorInfo
  interaction: InteractionStatusResp
}

export interface DanmakuItem {
  id: number
  content: string
  video_time: number
  color: string
  font_size: string
  mode: number // 0=滚动, 1=顶部, 2=底部
  user_id: string
  created_at: number
}

export interface VideoDraftStatusResp {
  status: 'draft' | 'transcoding' | 'pending_review' | 'published' | 'failed'
  fail_reason: string
  video_url: string
  cover_url: string
}

// 评论用户卡片
export interface CommentUserCard {
  id: string
  username: string
  avatar_url: string
  level?: number
}

// 评论（统一结构，适用于 video/article/dynamic）
export interface CommentItem {
  id: number
  user: CommentUserCard
  content: string
  like_count: number
  pinned: boolean
  created_at: string
  ip_location: string
  reply_count: number
  replies: CommentItem[]
}

// 兼容旧代码的别名
export type ArticleCommentItem = CommentItem

// 文章
export interface ArticleDetailResp {
  id: number
  title: string
  cover_url: string
  body_md: string
  images_json: string
  author: CommentUserCard
  view_count: number
  comment_count: number
  // 以下字段后端 ArticleDetailResp 未返回，保留为可选以兼容旧视图逻辑
  like_count?: number
  comments_closed?: boolean
  is_liked?: boolean
  is_favorited?: boolean
  tags: string[]
  created_at: string
}

// 投币
export interface CoinResultResp {
  added: number
  video_coin_cnt: number
  user_balance: number
}

export interface CoinLedgerItem {
  id: number
  delta_tenths: number
  reason_type: string
  reason?: string
  video_id: number
  created_at: string
}

// 收藏夹
export interface FavoriteFolder {
  id: number
  title: string
  cover_url: string
  video_count: number
  is_default: boolean
  created_at: string
}

export interface FavoriteFolderDetailResp {
  id: number
  title: string
  cover_url: string
  video_count: number
  is_default: boolean
}

// 关注
export interface FollowUserItem {
  id: string
  username: string
  avatar_url: string
  signature: string
  is_followed?: boolean
}

// 通知
export interface NotificationItem {
  id: number
  type: string
  related_id: string
  sender_names_json: string
  total_likes: number
  comment_preview: string
  payload_json: string
  is_read: boolean
  created_at: string
	// 前端解析字段
	sender_name?: string
	sender_avatar?: string
	content?: string
	// 评论类通知跳转用（来自 payload_json）
	target_type?: string
	target_id?: string
	comment_id?: string
}

export interface UnreadCountResp {
  count: number
}

// 私信
export interface MessageItem {
  id: number
  sender_id: string
  recipient_id: string
  content: string
  is_read: boolean
  created_at: string
}

export interface ConversationItem {
  peer_id: string
  peer_name: string
  peer_avatar: string
  last_content: string
  last_at: string
  unread_count: number
}

export interface MessageUnreadResp {
  count: number
}

// 历史
export interface VideoHistoryItem {
  video_id: number
  progress_sec: number
  duration_sec: number
  device: string
  viewed_at: string
  // 关联视频信息（后端 join 返回）
  id?: number
  title?: string
  cover_url?: string
  duration?: number
  watched_duration?: number
  up_name?: string
}

export interface ArticleHistoryItem {
  article_id: number
  device: string
  viewed_at: string
  // 关联文章信息（后端 join 返回）
  id?: number
  title?: string
  cover_url?: string
}

export interface SearchHistoryItem {
  keyword: string
  updated_at: string
  searched_at?: string
  id?: string
}

// 动态
export interface DynamicItem {
  id: number
  user_id: string
  username: string
  avatar_url: string
  type: string // dynamic | video | article
  title: string
  content: string
  images_json: string
  video_id?: number
  article_id?: number
  cover_url?: string
  duration?: number
  play_count?: number
  view_count?: number
  like_count: number
  comment_count: number
  is_liked: boolean
  created_at: string
}

// 每日任务
export interface DailyTaskData {
  id: number
  user_id: number
  task_date: string
  login_done: boolean
  watch_done: boolean
  created_at: string
  updated_at: string
}

export interface DailyTaskResp {
  level: UserLevelResp
  task: DailyTaskData
  today_exp: number
  // 展平字段，方便模板直接访问
  login_done: boolean
  watch_done: boolean
}

// 用户主页
export interface UserHomeResp {
  id: string
  username: string
  avatar_url: string
  signature: string
  address: string
  video_count: number
  birthday: string
  gender: string
  total_likes_received: number
  total_play_count: number
  experience: number
  fans_count: number
  following_count: number
  is_followed?: boolean
  favorite_folders: FavoriteFolderInfo[]
  videos: HomeVideoInfo[]
}

// 用户简档（私信入口按 user_id 拉取对端资料）
export interface UserBriefResp {
  id: string
  username: string
  avatar_url: string
}

export interface FavoriteFolderInfo {
  id: number
  title: string
  cover_url: string
  is_default: boolean
}

// 兼容旧代码
export type UserHomePageResp = UserHomeResp

// 验证码
export interface CaptchaResp {
  captcha_id: string
  pic_path: string
}
