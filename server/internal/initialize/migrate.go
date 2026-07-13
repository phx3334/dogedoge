package initialize

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"fake_tiktok/internal/domain/database"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB, lg *zap.Logger) error {
	if err := db.AutoMigrate(
		&database.Account{},
		&database.Video{},
		&database.Danmaku{},
		&database.Comment{},
		&database.CommentLike{},
		&database.VideoLike{},
		&database.FavoriteFolder{},
		&database.VideoFavorite{},
		&database.VideoCoin{},
		&database.UserFollow{},
		&database.Notification{},
		&database.LikeNotifMute{},
		&database.Article{},
		&database.ArticleFavorite{},
		&database.ArticleCoin{},
		&database.ArticleComment{},
		&database.ArticleCommentLike{},
		&database.Login{},
		&database.VideoViewHistory{},
		&database.ArticleViewHistory{},
		&database.UserDynamicText{},
		&database.UserDailyTask{},
		&database.UserSearchHistory{},
		&database.DynamicComment{},
		&database.DynamicCommentLike{},
		&database.DynamicCommentDislike{},
		&database.UserDynamicLike{},
		&database.CoinLedger{},
		&database.Message{},
	); err != nil {
		lg.Error("数据库迁移失败", zap.Error(err))
		return err
	}

	return AutoMigrateAll(db, lg)
}

func AutoMigrateAll(db *gorm.DB, lg *zap.Logger) error {
	migrations := []struct {
		name string
		fn   func(*gorm.DB, *zap.Logger) error
	}{
		{"backfillUserTotalLikesReceived", backfillUserTotalLikesReceived},
		{"backfillVideoCommentNotifications", backfillVideoCommentNotifications},
		{"backfillCommentReplyNotifications", backfillCommentReplyNotifications},
		{"migrateDefaultFavoriteFolder", migrateDefaultFavoriteFolder},
		{"backfillUserCoinBalance", backfillUserCoinBalance},
		{"backfillCoinLedger", backfillCoinLedger},
		{"migrateVideoFavoriteIndex", migrateVideoFavoriteIndex},
		{"migrateUserSearchHistory", migrateUserSearchHistory},
		{"backfillVideoCommentApproved", backfillVideoCommentApproved},
		{"backfillVideoCommentCount", backfillVideoCommentCount},
		{"backfillArticleCommentApproved", backfillArticleCommentApproved},
		{"backfillArticleCommentCount", backfillArticleCommentCount},
		{"backfillDynamicCommentApproved", backfillDynamicCommentApproved},
		{"backfillDynamicCommentCount", backfillDynamicCommentCount},
		{"migrateEnsureFieldColumns", migrateEnsureFieldColumns},
	}

	for _, m := range migrations {
		lg.Info("开始执行迁移", zap.String("migration", m.name))
		if err := m.fn(db, lg); err != nil {
			lg.Error("迁移失败", zap.String("migration", m.name), zap.Error(err))
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 场景1：用户收到点赞总数回填
// ---------------------------------------------------------------------------

func backfillUserTotalLikesReceived(db *gorm.DB, lg *zap.Logger) error {
	dialector := db.Dialector.Name()
	if dialector == "mysql" {
		return backfillUserTotalLikesReceivedMySQL(db, lg)
	}
	return backfillUserTotalLikesReceivedGORM(db, lg)
}

func backfillUserTotalLikesReceivedMySQL(db *gorm.DB, lg *zap.Logger) error {
	result := db.Exec(`
		UPDATE accounts
		SET total_likes_received = (
			COALESCE((SELECT COUNT(*) FROM comment_likes cl JOIN comments c ON cl.comment_id = c.id WHERE c.user_id = accounts.id), 0)
			+ COALESCE((SELECT COUNT(*) FROM video_likes vl JOIN videos v ON vl.video_id = v.id WHERE v.author_id = accounts.id), 0)
			+ COALESCE((SELECT COUNT(*) FROM article_favorites al JOIN articles ar ON al.article_id = ar.id WHERE ar.user_id = accounts.id), 0)
		)
		WHERE total_likes_received = 0
		  AND (
			EXISTS (SELECT 1 FROM comment_likes cl JOIN comments c ON cl.comment_id = c.id WHERE c.user_id = accounts.id)
			OR EXISTS (SELECT 1 FROM video_likes vl JOIN videos v ON vl.video_id = v.id WHERE v.author_id = accounts.id)
			OR EXISTS (SELECT 1 FROM article_favorites al JOIN articles ar ON al.article_id = ar.id WHERE ar.user_id = accounts.id)
		  )
	`)
	if result.Error != nil {
		return result.Error
	}
	lg.Info("backfillUserTotalLikesReceived 完成(MySQL)", zap.Int64("updated", result.RowsAffected))
	return nil
}

func backfillUserTotalLikesReceivedGORM(db *gorm.DB, lg *zap.Logger) error {
	var users []database.Account
	if err := db.Where("total_likes_received = ?", 0).Find(&users).Error; err != nil {
		return err
	}
	if len(users) == 0 {
		lg.Info("backfillUserTotalLikesReceived 已全部回填，无需处理")
		return nil
	}

	type likeCountResult struct {
		OwnerID string
		Count   int64
	}

	var commentCounts []likeCountResult
	if err := db.Table("comment_likes").
		Select("comments.user_id as owner_id, COUNT(*) as count").
		Joins("JOIN comments ON comment_likes.comment_id = comments.id").
		Group("comments.user_id").
		Scan(&commentCounts).Error; err != nil {
		return err
	}

	var videoCounts []likeCountResult
	if err := db.Table("video_likes").
		Select("accounts.id as owner_id, COUNT(*) as count").
		Joins("JOIN videos ON video_likes.video_id = videos.id").
		Joins("JOIN accounts ON videos.author_id = accounts.id").
		Group("accounts.id").
		Scan(&videoCounts).Error; err != nil {
		return err
	}

	var articleCounts []likeCountResult
	if err := db.Table("article_likes").
		Select("articles.user_id as owner_id, COUNT(*) as count").
		Joins("JOIN articles ON article_likes.article_id = articles.id").
		Group("articles.user_id").
		Scan(&articleCounts).Error; err != nil {
		return err
	}

	totalMap := make(map[string]int64)
	for _, c := range commentCounts {
		totalMap[c.OwnerID] += c.Count
	}
	for _, c := range videoCounts {
		totalMap[c.OwnerID] += c.Count
	}
	for _, c := range articleCounts {
		totalMap[c.OwnerID] += c.Count
	}

	updated := 0
	for _, u := range users {
		total := totalMap[u.ID]
		if total == 0 {
			continue
		}
		if err := db.Model(&database.Account{}).Where("id = ?", u.ID).
			Update("total_likes_received", total).Error; err != nil {
			return err
		}
		updated++
	}

	lg.Info("backfillUserTotalLikesReceived 完成", zap.Int("updated", updated))
	return nil
}

// ---------------------------------------------------------------------------
// 场景3：视频评论通知
// ---------------------------------------------------------------------------

func backfillVideoCommentNotifications(db *gorm.DB, lg *zap.Logger) error {
	var comments []database.Comment
	if err := db.Where("parent_id = ?", 0).Find(&comments).Error; err != nil {
		return err
	}
	if len(comments) == 0 {
		lg.Info("backfillVideoCommentNotifications 无顶级评论，无需处理")
		return nil
	}

	// 预加载视频作者映射
	videoIDs := make([]uint64, 0, len(comments))
	for _, c := range comments {
		videoIDs = append(videoIDs, c.VideoID)
	}

	var videos []database.Video
	if err := db.Where("id IN ?", videoIDs).Find(&videos).Error; err != nil {
		return err
	}

	// videoAuthorMap: video_id(数值) → author_id(字符串)
	videoAuthorMap := make(map[uint64]string)
	for _, v := range videos {
		videoAuthorMap[uint64(v.ID)] = v.AuthorID
	}

	// 查询已有通知去重
	type existKey struct {
		RecipientID string
		RelatedID   string
	}
	var existing []database.Notification
	if err := db.Where("type = ?", "video_comment_received").Find(&existing).Error; err != nil {
		return err
	}
	existSet := make(map[existKey]bool)
	for _, n := range existing {
		existSet[existKey{n.RecipientID, n.RelatedID}] = true
	}

	// 查询评论者用户名
	var commentUsers []database.Account
	commentUserIDs := make([]string, 0)
	for _, c := range comments {
		commentUserIDs = append(commentUserIDs, c.UserID)
	}
	if err := db.Where("id IN ?", commentUserIDs).Find(&commentUsers).Error; err != nil {
		return err
	}
	userNameMap := make(map[string]string)
	for _, u := range commentUsers {
		userNameMap[u.ID] = u.Username
	}

	created := 0
	for _, c := range comments {
		authorID := videoAuthorMap[c.VideoID]
		if authorID == "" || authorID == c.UserID {
			continue
		}
		key := existKey{authorID, strconv.FormatUint(c.ID, 10)}
		if existSet[key] {
			continue
		}

		senderName := userNameMap[c.UserID]
		namesJSON, _ := json.Marshal([]string{senderName})

		notif := database.Notification{
			RecipientID:     authorID,
			Type:            "video_comment_received",
			RelatedID:       strconv.FormatUint(c.ID, 10),
			SenderNamesJSON: string(namesJSON),
			CommentPreview:  truncate(c.Content, 32),
			IsRead:          false,
			CreatedAt:       c.CreatedAt,
			UpdatedAt:       c.CreatedAt,
		}
		if err := db.Create(&notif).Error; err != nil {
			return err
		}
		existSet[key] = true
		created++
	}

	lg.Info("backfillVideoCommentNotifications 完成", zap.Int("created", created))
	return nil
}

// ---------------------------------------------------------------------------
// 场景4：评论回复通知
// ---------------------------------------------------------------------------

func backfillCommentReplyNotifications(db *gorm.DB, lg *zap.Logger) error {
	var replies []database.Comment
	if err := db.Where("parent_id != ?", 0).Find(&replies).Error; err != nil {
		return err
	}
	if len(replies) == 0 {
		lg.Info("backfillCommentReplyNotifications 无回复评论，无需处理")
		return nil
	}

	// 查询被回复评论的作者
	parentIDs := make([]uint64, 0, len(replies))
	for _, r := range replies {
		parentIDs = append(parentIDs, r.ParentID)
	}
	var parentComments []database.Comment
	if err := db.Where("id IN ?", parentIDs).Find(&parentComments).Error; err != nil {
		return err
	}
	parentAuthorMap := make(map[uint64]string)
	for _, p := range parentComments {
		parentAuthorMap[p.ID] = p.UserID
	}

	// 已有通知去重
	type existKey struct {
		RecipientID string
		RelatedID   string
	}
	var existing []database.Notification
	if err := db.Where("type = ?", "reply_received").Find(&existing).Error; err != nil {
		return err
	}
	existSet := make(map[existKey]bool)
	for _, n := range existing {
		existSet[existKey{n.RecipientID, n.RelatedID}] = true
	}

	// 查询回复者用户名
	replyUserIDs := make([]string, 0, len(replies))
	for _, r := range replies {
		replyUserIDs = append(replyUserIDs, r.UserID)
	}
	var replyUsers []database.Account
	if err := db.Where("id IN ?", replyUserIDs).Find(&replyUsers).Error; err != nil {
		return err
	}
	userNameMap := make(map[string]string)
	for _, u := range replyUsers {
		userNameMap[u.ID] = u.Username
	}

	created := 0
	for _, r := range replies {
		parentAuthorID, ok := parentAuthorMap[r.ParentID]
		if !ok || parentAuthorID == r.UserID {
			continue
		}
		key := existKey{parentAuthorID, strconv.FormatUint(r.ID, 10)}
		if existSet[key] {
			continue
		}

		senderName := userNameMap[r.UserID]
		namesJSON, _ := json.Marshal([]string{senderName})

		notif := database.Notification{
			RecipientID:     parentAuthorID,
			Type:            "reply_received",
			RelatedID:       strconv.FormatUint(r.ID, 10),
			SenderNamesJSON: string(namesJSON),
			CommentPreview:  truncate(r.Content, 32),
			IsRead:          false,
			CreatedAt:       r.CreatedAt,
			UpdatedAt:       r.CreatedAt,
		}
		if err := db.Create(&notif).Error; err != nil {
			return err
		}
		existSet[key] = true
		created++
	}

	lg.Info("backfillCommentReplyNotifications 完成", zap.Int("created", created))
	return nil
}

// ---------------------------------------------------------------------------
// 场景5：默认收藏夹
// ---------------------------------------------------------------------------

func migrateDefaultFavoriteFolder(db *gorm.DB, lg *zap.Logger) error {
	type favUser struct {
		UserID string
	}
	var favUsers []favUser
	if err := db.Table("video_favorites").
		Select("DISTINCT user_id").
		Scan(&favUsers).Error; err != nil {
		return err
	}
	if len(favUsers) == 0 {
		lg.Info("migrateDefaultFavoriteFolder 无收藏记录，无需处理")
		return nil
	}

	var folderUsers []database.FavoriteFolder
	if err := db.Find(&folderUsers).Error; err != nil {
		return err
	}
	hasFolder := make(map[string]bool)
	for _, f := range folderUsers {
		hasFolder[f.UserID] = true
	}

	created := 0
	folderMap := make(map[string]uint64)
	for _, fu := range favUsers {
		if hasFolder[fu.UserID] {
			continue
		}
		folder := database.FavoriteFolder{
			UserID:    fu.UserID,
			Title:     "默认收藏夹",
			IsDefault: true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := db.Create(&folder).Error; err != nil {
			return err
		}
		folderMap[fu.UserID] = folder.ID
		hasFolder[fu.UserID] = true
		created++
	}

	var defaultFolders []database.FavoriteFolder
	if err := db.Where("is_default = ?", true).Find(&defaultFolders).Error; err != nil {
		return err
	}
	for _, f := range defaultFolders {
		folderMap[f.UserID] = f.ID
	}

	dialector := db.Dialector.Name()
	var migrated int64
	if dialector == "mysql" {
		for userID, folderID := range folderMap {
			result := db.Exec(
				"UPDATE video_favorites SET folder_id = ? WHERE user_id = ? AND folder_id = 0",
				folderID, userID,
			)
			if result.Error != nil {
				return result.Error
			}
			migrated += result.RowsAffected
		}
	} else {
		var orphans []database.VideoFavorite
		if err := db.Where("folder_id = ?", 0).Find(&orphans).Error; err != nil {
			return err
		}
		for _, o := range orphans {
			fid, ok := folderMap[o.UserID]
			if !ok {
				continue
			}
			if err := db.Model(&database.VideoFavorite{}).Where("id = ?", o.ID).
				Update("folder_id", fid).Error; err != nil {
				return err
			}
			migrated++
		}
	}

	lg.Info("migrateDefaultFavoriteFolder 完成",
		zap.Int("folders_created", created),
		zap.Int64("favorites_migrated", migrated),
	)
	return nil
}

// ---------------------------------------------------------------------------
// 场景6：用户硬币余额默认值
// ---------------------------------------------------------------------------

func backfillUserCoinBalance(db *gorm.DB, lg *zap.Logger) error {
	dialector := db.Dialector.Name()
	if dialector == "mysql" {
		result := db.Exec("UPDATE accounts SET coin_balance_tenths = 210 WHERE coin_balance_tenths = 0")
		if result.Error != nil {
			return result.Error
		}
		lg.Info("backfillUserCoinBalance 完成(MySQL)", zap.Int64("updated", result.RowsAffected))
		return nil
	}

	var users []database.Account
	if err := db.Where("coin_balance_tenths = ?", 0).Find(&users).Error; err != nil {
		return err
	}
	if len(users) == 0 {
		lg.Info("backfillUserCoinBalance 已全部回填，无需处理")
		return nil
	}

	updated := 0
	for _, u := range users {
		if err := db.Model(&database.Account{}).Where("id = ?", u.ID).
			Update("coin_balance_tenths", 230).Error; err != nil {
			return err
		}
		updated++
	}
	lg.Info("backfillUserCoinBalance 完成", zap.Int("updated", updated))
	return nil
}

// ---------------------------------------------------------------------------
// 场景7：硬币流水记录
// ---------------------------------------------------------------------------

func backfillCoinLedger(db *gorm.DB, lg *zap.Logger) error {
	type ledgerKey struct {
		UserID     uint64
		ReasonType string
		VideoID    uint64
		CreatedAt  time.Time
	}
	var existing []database.CoinLedger
	if err := db.Find(&existing).Error; err != nil {
		return err
	}
	existSet := make(map[ledgerKey]bool)
	for _, e := range existing {
		existSet[ledgerKey{e.UserID, e.ReasonType, e.VideoID, e.CreatedAt}] = true
	}

	created := 0

	var videoCoins []database.VideoCoin
	if err := db.Find(&videoCoins).Error; err != nil {
		return err
	}
	for _, vc := range videoCoins {
		delta := int64(vc.Amount) * -10
		vcUserID, _ := strconv.ParseUint(vc.UserID, 10, 64)
		key := ledgerKey{vcUserID, "coin_video", vc.VideoID, vc.CreatedAt}
		if existSet[key] {
			continue
		}
		ledger := database.CoinLedger{
			UserID:      vcUserID,
			DeltaTenths: delta,
			ReasonType:  "coin_video",
			VideoID:     vc.VideoID,
			CreatedAt:   vc.CreatedAt,
		}
		if err := db.Create(&ledger).Error; err != nil {
			return err
		}
		existSet[key] = true
		created++
	}

	var dailyTasks []database.UserDailyTask
	if err := db.Find(&dailyTasks).Error; err != nil {
		return err
	}
	for _, dt := range dailyTasks {
		if dt.LoginDone {
			key := ledgerKey{dt.UserID, "daily_login", 0, dt.CreatedAt}
			if !existSet[key] {
				ledger := database.CoinLedger{
					UserID:      dt.UserID,
					DeltaTenths: 5,
					ReasonType:  "daily_login",
					VideoID:     0,
					CreatedAt:   dt.CreatedAt,
				}
				if err := db.Create(&ledger).Error; err != nil {
					return err
				}
				existSet[key] = true
				created++
			}
		}
		if dt.WatchDone {
			key := ledgerKey{dt.UserID, "daily_watch", 0, dt.CreatedAt}
			if !existSet[key] {
				ledger := database.CoinLedger{
					UserID:      dt.UserID,
					DeltaTenths: 5,
					ReasonType:  "daily_watch",
					VideoID:     0,
					CreatedAt:   dt.CreatedAt,
				}
				if err := db.Create(&ledger).Error; err != nil {
					return err
				}
				existSet[key] = true
				created++
			}
		}
	}

	lg.Info("backfillCoinLedger 完成", zap.Int("created", created))
	return nil
}

// ---------------------------------------------------------------------------
// 场景8：视频收藏索引迁移
// ---------------------------------------------------------------------------

func migrateVideoFavoriteIndex(db *gorm.DB, lg *zap.Logger) error {
	dialector := db.Dialector.Name()
	migrator := db.Migrator()

	if dialector == "mysql" {
		if migrator.HasIndex(&database.VideoFavorite{}, "idx_video_fav_user_video") {
			if err := db.Exec("ALTER TABLE video_favorites DROP INDEX idx_video_fav_user_video").Error; err != nil {
				return err
			}
		}
		if !migrator.HasIndex(&database.VideoFavorite{}, "idx_video_fav_user_video_folder") {
			if err := db.Exec("ALTER TABLE video_favorites ADD UNIQUE INDEX idx_video_fav_user_video_folder (user_id, video_id, folder_id)").Error; err != nil {
				return err
			}
		}
		lg.Info("migrateVideoFavoriteIndex 完成(MySQL)")
		return nil
	}

	if migrator.HasIndex(&database.VideoFavorite{}, "idx_video_fav_user_video") {
		if err := migrator.DropIndex(&database.VideoFavorite{}, "idx_video_fav_user_video"); err != nil {
			return err
		}
	}
	if !migrator.HasIndex(&database.VideoFavorite{}, "idx_video_fav_user_video_folder") {
		if err := migrator.CreateIndex(&database.VideoFavorite{}, "idx_video_fav_user_video_folder"); err != nil {
			return err
		}
	}
	lg.Info("migrateVideoFavoriteIndex 完成(SQLite)")
	return nil
}

// ---------------------------------------------------------------------------
// 场景9：搜索历史迁移
// ---------------------------------------------------------------------------

func migrateUserSearchHistory(db *gorm.DB, lg *zap.Logger) error {
	migrator := db.Migrator()

	const idxName = "idx_user_search_user_keyword"

	if migrator.HasIndex(&database.UserSearchHistory{}, idxName) {
		lg.Info("migrateUserSearchHistory 索引已存在，跳过")
		return nil
	}

	dialector := db.Dialector.Name()
	if dialector == "mysql" {
		if err := db.Exec(`
			DELETE h1 FROM user_search_histories h1
			INNER JOIN user_search_histories h2
			ON h1.user_id = h2.user_id AND h1.keyword_norm = h2.keyword_norm
			AND h1.id < h2.id
		`).Error; err != nil {
			return err
		}
	} else {
		if err := db.Exec(`
			DELETE FROM user_search_histories
			WHERE id NOT IN (
				SELECT max_id FROM (
					SELECT MAX(id) as max_id FROM user_search_histories
					GROUP BY user_id, keyword_norm
				)
			)
		`).Error; err != nil {
			return err
		}
	}

	if err := db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX %s ON user_search_histories (user_id, keyword_norm)", idxName,
	)).Error; err != nil {
		return err
	}

	lg.Info("migrateUserSearchHistory 完成")
	return nil
}

// ---------------------------------------------------------------------------
// 场景12：视频评论精选状态
// ---------------------------------------------------------------------------

func backfillVideoCommentApproved(db *gorm.DB, lg *zap.Logger) error {
	dialector := db.Dialector.Name()
	if dialector == "mysql" {
		// Video 实体没有 comments_curated 字段（视频无精选模式），直接把所有 approved=0 的评论设为 1
		result := db.Exec(`
			UPDATE comments c
			SET c.approved = 1
			WHERE c.approved = 0 AND c.video_id > 0
		`)
		if result.Error != nil {
			return result.Error
		}
		lg.Info("backfillVideoCommentApproved 完成(MySQL)", zap.Int64("updated", result.RowsAffected))
		return nil
	}

	// Video 实体无 comments_curated 字段，直接把所有 approved=0 的评论设为 1
	result := db.Model(&database.Comment{}).
		Where("video_id > ? AND approved = ?", 0, false).
		Update("approved", true)
	if result.Error != nil {
		return result.Error
	}
	lg.Info("backfillVideoCommentApproved 完成", zap.Int64("updated", result.RowsAffected))
	return nil
}

// ---------------------------------------------------------------------------
// 场景13：视频评论数统计
// ---------------------------------------------------------------------------

func backfillVideoCommentCount(db *gorm.DB, lg *zap.Logger) error {
	dialector := db.Dialector.Name()
	if dialector == "mysql" {
		// Video 实体无 comments_curated 字段（视频无精选模式），直接统计所有已审核评论
		result := db.Exec(`
			UPDATE videos v
			SET comments_count = (
				SELECT COUNT(*) FROM comments c
				WHERE c.video_id = v.id AND c.approved = 1
			)
		`)
		if result.Error != nil {
			return result.Error
		}
		lg.Info("backfillVideoCommentCount 完成(MySQL)", zap.Int64("updated", result.RowsAffected))
		return nil
	}

	// 非 MySQL：直接按 video 分组统计
	type videoCount struct {
		VideoID uint64
		Count   int64
	}
	var rows []videoCount
	if err := db.Model(&database.Comment{}).
		Select("video_id, COUNT(*) as count").
		Where("approved = ?", true).
		Group("video_id").Scan(&rows).Error; err != nil {
		return err
	}
	updated := 0
	for _, r := range rows {
		if err := db.Model(&database.Video{}).Where("id = ?", r.VideoID).
			Update("comments_count", r.Count).Error; err != nil {
			return err
		}
		updated++
	}
	lg.Info("backfillVideoCommentCount 完成", zap.Int("updated", updated))
	return nil
}

// ---------------------------------------------------------------------------
// 场景14：专栏评论精选状态
// ---------------------------------------------------------------------------

func backfillArticleCommentApproved(db *gorm.DB, lg *zap.Logger) error {
	dialector := db.Dialector.Name()
	if dialector == "mysql" {
		result := db.Exec(`
			UPDATE article_comments ac
			JOIN articles ar ON ac.article_id = ar.id
			SET ac.approved = 1
			WHERE ar.comments_curated = 0 AND ac.approved = 0
		`)
		if result.Error != nil {
			return result.Error
		}
		lg.Info("backfillArticleCommentApproved 完成(MySQL)", zap.Int64("updated", result.RowsAffected))
		return nil
	}

	var articles []database.Article
	if err := db.Where("comments_curated = ?", false).Find(&articles).Error; err != nil {
		return err
	}
	if len(articles) == 0 {
		lg.Info("backfillArticleCommentApproved 无需处理")
		return nil
	}

	articleIDs := make([]uint64, 0, len(articles))
	for _, a := range articles {
		articleIDs = append(articleIDs, a.ID)
	}

	result := db.Model(&database.ArticleComment{}).
		Where("article_id IN ? AND approved = ?", articleIDs, false).
		Update("approved", true)
	if result.Error != nil {
		return result.Error
	}
	lg.Info("backfillArticleCommentApproved 完成", zap.Int64("updated", result.RowsAffected))
	return nil
}

// ---------------------------------------------------------------------------
// 场景15：专栏评论数统计
// ---------------------------------------------------------------------------

func backfillArticleCommentCount(db *gorm.DB, lg *zap.Logger) error {
	dialector := db.Dialector.Name()
	if dialector == "mysql" {
		result := db.Exec(`
			UPDATE articles ar
			SET comment_count = (
				SELECT COUNT(*) FROM article_comments ac
				WHERE ac.article_id = ar.id AND ac.approved = 1
			)
			WHERE ar.comments_curated = 1
		`)
		if result.Error != nil {
			return result.Error
		}
		lg.Info("backfillArticleCommentCount 完成(MySQL)", zap.Int64("updated", result.RowsAffected))
		return nil
	}

	var articles []database.Article
	if err := db.Where("comments_curated = ?", true).Find(&articles).Error; err != nil {
		return err
	}
	if len(articles) == 0 {
		lg.Info("backfillArticleCommentCount 无需处理")
		return nil
	}

	updated := 0
	for _, a := range articles {
		var count int64
		if err := db.Model(&database.ArticleComment{}).
			Where("article_id = ? AND approved = ?", a.ID, true).
			Count(&count).Error; err != nil {
			return err
		}
		if err := db.Model(&database.Article{}).Where("id = ?", a.ID).
			Update("comment_count", count).Error; err != nil {
			return err
		}
		updated++
	}
	lg.Info("backfillArticleCommentCount 完成", zap.Int("updated", updated))
	return nil
}

// ---------------------------------------------------------------------------
// 场景16：动态评论精选状态
// ---------------------------------------------------------------------------

func backfillDynamicCommentApproved(db *gorm.DB, lg *zap.Logger) error {
	// DynamicComment 实体无 approved / curated_ignored 字段，动态评论无精选模式，无需处理
	lg.Info("backfillDynamicCommentApproved 无需处理（动态无精选模式）")
	return nil
}

// ---------------------------------------------------------------------------
// 场景17：动态评论数统计
// ---------------------------------------------------------------------------

func backfillDynamicCommentCount(db *gorm.DB, lg *zap.Logger) error {
	// 使用 GORM API（不依赖表名硬编码），统计所有评论数
	type dynCount struct {
		DynamicID uint64
		Count     int64
	}
	var rows []dynCount
	if err := db.Model(&database.DynamicComment{}).
		Select("dynamic_id, COUNT(*) as count").
		Group("dynamic_id").Scan(&rows).Error; err != nil {
		return err
	}
	updated := 0
	for _, r := range rows {
		if err := db.Model(&database.UserDynamicText{}).Where("id = ?", r.DynamicID).
			Update("comment_count", r.Count).Error; err != nil {
			return err
		}
		updated++
	}
	lg.Info("backfillDynamicCommentCount 完成", zap.Int("updated", updated))
	return nil
}

// ---------------------------------------------------------------------------
// 场景18：播放和评论字段确保
// ---------------------------------------------------------------------------

func migrateEnsureFieldColumns(db *gorm.DB, lg *zap.Logger) error {
	type colCheck struct {
		table  string
		column string
	}

	checks := []colCheck{
		{"videos", "play_count"},
		{"videos", "comments_count"},
		// Video 实体无 comments_curated 字段（视频无精选模式），不检查
		{"videos", "danmaku_closed"},
		{"comments", "approved"},
		{"comments", "curated_ignored"},
		{"comments", "ip_location"},
		{"articles", "comment_count"},
		{"articles", "comments_curated"},
		{"article_comments", "approved"},
		{"article_comments", "curated_ignored"},
		{"article_comments", "ip_location"},
		{"user_dynamics", "comment_count"},
		// UserDynamicText 实体无 comments_curated 字段（动态无精选模式），不检查
		// DynamicComment 实体无 approved/curated_ignored 字段，不检查
		{"danmakus", "font_size"},
	}

	migrator := db.Migrator()
	ensured := 0
	models := []interface{}{
		&database.Video{},
		&database.Comment{},
		&database.Article{},
		&database.ArticleComment{},
		&database.UserDynamicText{},
		&database.DynamicComment{},
		&database.Danmaku{},
	}
	for _, c := range checks {
		if !migrator.HasColumn(c.table, c.column) {
			if err := db.AutoMigrate(models...); err != nil {
				return fmt.Errorf("ensure column %s.%s failed: %w", c.table, c.column, err)
			}
			ensured++
			lg.Info("migrateEnsureFieldColumns 添加字段", zap.String("table", c.table), zap.String("column", c.column))
		}
	}

	lg.Info("migrateEnsureFieldColumns 完成", zap.Int("ensured", ensured))
	return nil
}

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
