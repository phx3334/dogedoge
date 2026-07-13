package handler

import (
	"fake_tiktok/internal/logic"

	"go.uber.org/zap"
)

type HandlerGroup struct {
	UserHandler
	BaseHandler
	VideoHandler
	SearchHandler
	InteractionHandler
	VideoDraftHandler VideoDraftHandler
	CommentHandler    CommentHandler
	ArticleHandler    ArticleHandler

	// P1-P3 个人中心相关 Handler
	CoinHandler           CoinHandler
	FavoriteFolderHandler FavoriteFolderHandler
	FollowHandler         FollowHandler
	NotificationHandler   NotificationHandler
	HistoryHandler        HistoryHandler
	DynamicHandler        DynamicHandler
	DailyTaskHandler      DailyTaskHandler
	UserHomeHandler       UserHomeHandler

	// 用户间私信
	MessageHandler MessageHandler

	// AI 聊天
	AIHandler AIHandler
}

type HandlerDeps struct {
	Logic  *logic.LogicGroup
	Logger *zap.Logger
}

func NewHandlerGroup(deps *HandlerDeps) *HandlerGroup {
	return &HandlerGroup{
		UserHandler:        UserHandler{logic: deps.Logic, logger: deps.Logger},
		BaseHandler:        BaseHandler{logic: deps.Logic, logger: deps.Logger},
		VideoHandler:       VideoHandler{logic: deps.Logic, logger: deps.Logger},
		SearchHandler:      SearchHandler{logic: deps.Logic, logger: deps.Logger},
		InteractionHandler: InteractionHandler{logic: deps.Logic, logger: deps.Logger},
		VideoDraftHandler:  VideoDraftHandler{logic: deps.Logic, logger: deps.Logger},
		CommentHandler:     CommentHandler{logic: deps.Logic, logger: deps.Logger},
		ArticleHandler:     ArticleHandler{logic: deps.Logic, logger: deps.Logger},

		// P1-P3 个人中心相关 Handler
		CoinHandler:           CoinHandler{logic: deps.Logic, logger: deps.Logger},
		FavoriteFolderHandler: FavoriteFolderHandler{logic: deps.Logic, logger: deps.Logger},
		FollowHandler:         FollowHandler{logic: deps.Logic, logger: deps.Logger},
		NotificationHandler:   NotificationHandler{logic: deps.Logic, logger: deps.Logger},
		HistoryHandler:        HistoryHandler{logic: deps.Logic, logger: deps.Logger},
		DynamicHandler:        DynamicHandler{logic: deps.Logic, logger: deps.Logger},
		DailyTaskHandler:      DailyTaskHandler{logic: deps.Logic, logger: deps.Logger},
		UserHomeHandler:       UserHomeHandler{logic: deps.Logic, logger: deps.Logger},

		MessageHandler: MessageHandler{logic: deps.Logic, logger: deps.Logger},

		AIHandler: AIHandler{logic: deps.Logic, logger: deps.Logger},
	}
}
