package routers

type RouterGroup struct {
	UserRouter
	BaseRouter
	VideoRouter
	SearchRouter
	CommentRouter
	ArticleRouter
	InteractionRouter

	// P1-P3 个人中心相关 Router
	CoinRouter
	FavoriteFolderRouter
	FollowRouter
	NotificationRouter
	HistoryRouter
	DynamicRouter
	DailyTaskRouter
	UserHomeRouter
	AIRouter
	MessageRouter
}

// RouterGroupApp 是全局路由组实例，供 initialize 包引用。
// 修正了旧版拼写错误 Rounter -> Router。
var RouterGroupApp = new(RouterGroup)
