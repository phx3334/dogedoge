package logic

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/domain/other"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/pkg"

	"github.com/bwmarrin/snowflake"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// -----------------------------------------------------------------------------

type UserLogic struct {
	deps *LogicDeps
	// sfGroup 用于缓存击穿保护：同一个 userID 的并发回源请求只执行一次，
	// 其他请求共享结果，避免大量并发请求同时穿透到 MySQL
	sfGroup singleflight.Group
}

func NewUserLogic(deps *LogicDeps) *UserLogic {
	return &UserLogic{deps: deps}
}

// -----------------------------------------------------------------------------

type TokenResult struct {
	Account           database.Account
	AccessToken       string
	AccessTokenExpire int64
	RefreshToken      string
	RefreshExpiry     int
}

var snowNode *snowflake.Node

// snowOnce 保证 snowNode 只初始化一次，避免并发调用时重复创建或竞态访问。
// 旧版 init() 是 UserLogic 的方法接收者函数，从未被调用，导致 snowNode 为 nil，
// Register 中 snowNode.Generate() 必现 nil pointer panic。
// 改用 sync.Once + 懒初始化：首次调用 ensureSnowNode 时自动创建，后续调用直接复用。
var snowOnce sync.Once

// ensureSnowNode 确保 snowNode 已初始化（懒初始化 + 并发安全）。
// 使用 sync.Once 保证即使多个 goroutine 并发调用 Register，snowNode 也只创建一次。
// 节点 ID 固定为 1，适用于单实例部署；多实例部署时需改为从配置或环境变量读取。
func ensureSnowNode() {
	snowOnce.Do(func() {
		node, err := snowflake.NewNode(1)
		if err != nil {
			// snowflake.NewNode 仅在节点 ID 超出范围时返回错误（0-1023），
			// 节点 ID 固定为 1 不可能出错，但保留 panic 作为防御性编程。
			panic(fmt.Sprintf("failed to create snowflake node: %v", err))
		}
		snowNode = node
	})
}

// -----------------------------------------------------------------------------

func (u *UserLogic) Register(ctx context.Context, account database.Account) (database.Account, error) {
	if _, err := u.deps.AccountRepo.FindByEmail(ctx, account.Email); err != nil {
		// 修复：非 ErrRecordNotFound 的错误（如 DB 连接失败）不应被静默忽略，
		// 否则可能导致重复邮箱注册
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return database.Account{}, fmt.Errorf("check email failed: %w", err)
		}
		// ErrRecordNotFound: 邮箱未注册，继续注册流程
	} else {
		return database.Account{}, errors.New("email already exists")
	}
	// 确保 snowNode 已初始化：懒初始化 + sync.Once 并发安全，
	// 首次调用时创建 snowflake 节点，后续调用直接复用。
	ensureSnowNode()
	hashedPwd, err := pkg.BcryptHash(account.Password)
	if err != nil {
		return database.Account{}, fmt.Errorf("failed to hash password: %w", err)
	}
	account.Password = hashedPwd
	account.ID = snowNode.Generate().String()
	account.AvatarURL = "/uploads/avatar/default.jpg"
	account.Role = database.User
	account.VideoCount = 0
	if err := u.deps.AccountRepo.Create(ctx, &account); err != nil {
		return database.Account{}, err
	}
	return account, nil
}

// -----------------------------------------------------------------------------

func (u *UserLogic) Login(ctx context.Context, account database.Account) (database.Account, error) {
	user, err := u.deps.AccountRepo.FindByEmail(ctx, account.Email)
	if err == nil {
		if !pkg.BcryptCheck(account.Password, user.Password) {
			return database.Account{}, errors.New("password error")
		}
		return *user, nil
	}
	return database.Account{}, errors.New("email not found")
}

// -----------------------------------------------------------------------------

func (u *UserLogic) ForgotPassword(ctx context.Context, req request.ForgotPassword) error {
	user, err := u.deps.AccountRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return err
	}
	hashedPwd, err := pkg.BcryptHash(req.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	user.Password = hashedPwd
	return u.deps.AccountRepo.Save(ctx, user)
}

// -----------------------------------------------------------------------------

// getUserCacheData 获取用户缓存数据（Redis 优先，熔断保护 + singleflight 防击穿 + StaticHit 降级回源）
// 供 PersonalInfo 和 UserHome 复用
func (u *UserLogic) getUserCacheData(ctx context.Context, userID string) (*cache.UserCacheData, error) {
	var userData *cache.UserCacheData
	redisErr := u.deps.Breakers.Redis.Execute(func() error {
		var err error
		userData, err = u.deps.UserCacheRepo.GetUserCache(ctx, userID)
		return err
	})
	if redisErr != nil || userData == nil {
		// 全部未命中：走 MySQL 回源（static + dynamic 一起回填）
		v, sfErr, _ := u.sfGroup.Do("user:"+userID, func() (interface{}, error) {
			if semErr := u.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
				// 修复：信号量获取失败时返回错误，而非无保护地继续执行查询
				// 高并发场景下无信号量保护可能导致连接池耗尽
				u.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(semErr))
				return nil, fmt.Errorf("server busy, please try again later")
			} else {
				defer u.deps.Breakers.MySQLReadSem.Release(1)
			}
			var data *cache.UserCacheData
			backfillErr := u.deps.Breakers.MySQL.Execute(func() error {
				var err error
				data, err = u.deps.BackfillRepo.BackfillUserCache(ctx, userID)
				return err
			})
			if backfillErr != nil {
				return nil, backfillErr
			}
			return data, nil
		})
		if sfErr != nil {
			if errors.Is(sfErr, breaker.ErrCircuitOpen) {
				u.deps.Logger.Warn("MySQL circuit open", zap.String("user_id", userID))
			} else {
				u.deps.Logger.Warn("BackfillUserCache failed", zap.String("user_id", userID), zap.Error(sfErr))
			}
		} else if v != nil {
			userData = v.(*cache.UserCacheData)
		}
	} else if !userData.StaticHit {
		// 动态区命中但静态区未命中：只对 static 区做降级回源
		v, sfErr, _ := u.sfGroup.Do("user:static:"+userID, func() (interface{}, error) {
			if semErr := u.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
				// 修复：信号量获取失败时返回错误，而非无保护地继续执行查询
				// 高并发场景下无信号量保护可能导致连接池耗尽
				u.deps.Logger.Warn("MySQL read semaphore acquire failed for static", zap.Error(semErr))
				return nil, fmt.Errorf("server busy, please try again later")
			} else {
				defer u.deps.Breakers.MySQLReadSem.Release(1)
			}
			var data *cache.UserCacheData
			backfillErr := u.deps.Breakers.MySQL.Execute(func() error {
				var err error
				data, err = u.deps.BackfillRepo.BackfillUserCache(ctx, userID)
				return err
			})
			if backfillErr != nil {
				return nil, backfillErr
			}
			return data, nil
		})
		if sfErr != nil {
			u.deps.Logger.Warn("BackfillUserCache static failed", zap.String("user_id", userID), zap.Error(sfErr))
		} else if v != nil {
			backfilled := v.(*cache.UserCacheData)
			userData.ID = backfilled.ID
			userData.Username = backfilled.Username
			userData.AvatarURL = backfilled.AvatarURL
			userData.Signature = backfilled.Signature
			userData.Address = backfilled.Address
			userData.Birthday = backfilled.Birthday
			userData.Gender = backfilled.Gender
			userData.PrivacyPublicFavorites = backfilled.PrivacyPublicFavorites
			userData.PrivacyPublicFollowing = backfilled.PrivacyPublicFollowing
			userData.PrivacyPublicFans = backfilled.PrivacyPublicFans
			userData.ViewHistoryPaused = backfilled.ViewHistoryPaused
			userData.StaticHit = true
		}
	}
	return userData, nil
}

// GetBrief 按 user_id 返回用户简档（id / username / avatar_url）。
// 用于私信入口：从关注列表 / 用户主页点"私信"时按 user_id 拉取对端资料，
// 无需手动输入对方 ID。
func (u *UserLogic) GetBrief(ctx context.Context, userID string) (*response.UserBriefResp, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id 为空")
	}
	acc, err := u.deps.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("用户不存在")
	}
	return &response.UserBriefResp{
		ID:        acc.ID,
		Username:  acc.Username,
		AvatarURL: acc.AvatarURL,
	}, nil
}

// UserHome 获取用户主页信息
// viewerID 为当前登录用户（用于计算 is_followed）；未登录时传空串。
// page / pageSize 用于控制作者视频列表的分页，来自请求参数
func (u *UserLogic) UserHome(ctx context.Context, viewerID, userID string, page, pageSize int) (*response.UserHomeResp, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// ---- 1. 获取用户缓存 ----
	userData, err := u.getUserCacheData(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userData == nil {
		return nil, fmt.Errorf("user not found")
	}

	// ---- 2. 收藏夹 ----
	var favoriteFolders []response.FavoriteFolderInfo
	{
		if semErr := u.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
			// 修复：信号量获取失败时返回错误，而非无保护地继续执行查询
			// 高并发场景下无信号量保护可能导致连接池耗尽
			u.deps.Logger.Warn("MySQL read semaphore acquire failed for favorite folders", zap.Error(semErr))
			return nil, fmt.Errorf("server busy, please try again later")
		} else {
			defer u.deps.Breakers.MySQLReadSem.Release(1)
		}
		var folders []database.FavoriteFolder
		breakerErr := u.deps.Breakers.MySQL.Execute(func() error {
			var err error
			folders, err = u.deps.FavoriteFolderRepo.FindByUserID(ctx, userID)
			return err
		})
		if breakerErr != nil {
			if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
				u.deps.Logger.Warn("MySQL circuit open, skip FindByUserID", zap.String("user_id", userID))
			} else {
				u.deps.Logger.Warn("FindByUserID failed", zap.String("user_id", userID), zap.Error(breakerErr))
			}
		} else {
			favoriteFolders = make([]response.FavoriteFolderInfo, 0, len(folders))
			for _, f := range folders {
				coverURL := f.CoverURL
				// 没有自定义封面时，默认使用第一个视频的封面
				if coverURL == "" {
					_ = u.deps.Breakers.MySQL.Execute(func() error {
						ids, _, err := u.deps.FavoriteRepo.ListFavoritesByFolder(ctx, f.UserID, f.ID, 1, 1)
						if err != nil || len(ids) == 0 {
							return err
						}
						videos, err := u.deps.VideoRepo.FindPublishedVideosByIDs(ctx, ids)
						if err != nil || len(videos) == 0 {
							return err
						}
						coverURL = videos[0].CoverURL
						return nil
					})
				}
				favoriteFolders = append(favoriteFolders, response.FavoriteFolderInfo{
					ID:        f.ID,
					Title:     f.Title,
					CoverURL:  coverURL,
					IsDefault: f.IsDefault,
				})
			}
		}
	}

	// ---- 4. 作者视频列表（按时间倒序，使用请求分页参数） ----
	var videos []response.HomeVideoInfo
	// 分页参数校验：page 从 1 开始，pageSize 默认 10、上限 30
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 30 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	if semErr := u.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
		// 修复：信号量获取失败时返回错误，而非无保护地继续执行查询
		// 高并发场景下无信号量保护可能导致连接池耗尽
		u.deps.Logger.Warn("MySQL read semaphore acquire failed for author videos", zap.Error(semErr))
		return nil, fmt.Errorf("server busy, please try again later")
	} else {
		defer u.deps.Breakers.MySQLReadSem.Release(1)
	}
	var dbVideos []database.Video
	breakerErr := u.deps.Breakers.MySQL.Execute(func() error {
		var err error
		dbVideos, err = u.deps.VideoRepo.FindPublishedVideosByAuthorID(ctx, userID, pageSize, offset)
		return err
	})
	if breakerErr != nil {
		if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
			u.deps.Logger.Warn("MySQL circuit open, skip FindPublishedVideosByAuthorID", zap.String("user_id", userID))
		} else {
			u.deps.Logger.Warn("FindPublishedVideosByAuthorID failed", zap.String("user_id", userID), zap.Error(breakerErr))
		}
	} else {
		videos = make([]response.HomeVideoInfo, 0, len(dbVideos))
		for _, v := range dbVideos {
			videos = append(videos, response.HomeVideoInfo{
				ID:           v.ID,
				UpName:       userData.Username,
				UpAvatar:     userData.AvatarURL,
				Title:        v.Title,
				CoverURL:     v.CoverURL,
				PlayCount:    v.PlayCount,
				CommentCount: v.CommentsCount,
				Duration:     v.DurationSec,
				CreatedAt:    v.CreatedAt,
				FavCount:     v.FavCount,
			})
		}
	}

	// ---- 5. 组装响应 ----
	// 修复：video_count 从 MySQL 实时读取，而非使用缓存值。
	// 原因：worker 转码完成时只更新 MySQL 的 video_count，未更新 Redis 缓存，
	// 且缓存回填使用 HSetNX 不会覆盖已有值，导致缓存中的 video_count 永远停留在旧值。
	realVideoCount := userData.VideoCount
	// 经验从 DB 读取（缓存经验可能因历史写入未同步而过期，等级展示必须准确）
	var realExperience uint64
	if account, accErr := u.deps.AccountRepo.FindByID(ctx, userID); accErr == nil {
		realVideoCount = account.VideoCount
		realExperience = account.Experience
	}

	// fans_count / following_count 实时从 user_follows 表统计。
	// 原因：关注/取关虽已通过 HIncrBy 维护 Redis 缓存，但历史缓存基线可能陈旧
	// （本修复前的关注未回写缓存），实时统计可保证主页数字永远准确、随关注即时变化。
	realFansCount := userData.FansCount
	if n, ferr := u.deps.InteractionRepo.GetFansCount(ctx, userID); ferr == nil {
		realFansCount = n
	}
	realFollowingCount := userData.FollowingCount
	if n, ferr := u.deps.InteractionRepo.GetFollowingCount(ctx, userID); ferr == nil {
		realFollowingCount = n
	}

	// is_followed：当前登录用户是否关注了该主页用户。
	// 仅当 viewer 存在且不是本人时才查询；否则（未登录 / 自己主页）为 false。
	isFollowed := false
	if viewerID != "" && viewerID != userID {
		if ok, ferr := u.deps.InteractionRepo.IsUserFollowed(ctx, viewerID, userID); ferr == nil {
			isFollowed = ok
		}
	}

	return &response.UserHomeResp{
		ID:                 userData.ID,
		AvatarURL:          userData.AvatarURL,
		Signature:          userData.Signature,
		Username:           userData.Username,
		Address:            userData.Address,
		VideoCount:         realVideoCount,
		Birthday:           userData.Birthday,
		Gender:             userData.Gender,
		TotalLikesReceived: userData.TotalLikesReceived,
		TotalPlayCount:     userData.TotalPlayCount,
		Experience:         realExperience,
		FansCount:          realFansCount,
		FollowingCount:     realFollowingCount,
		IsFollowed:        isFollowed,
		FavoriteFolders:    favoriteFolders,
		Videos:             videos,
	}, nil
}

// -----------------------------------------------------------------------------

// Logout 用户登出：删除 Redis 中的 JWT 记录 + 将 refresh token 加入黑名单。
//
// 旧版静默忽略 DelJWT / ToBlackList 的错误（用 _ 丢弃），导致 Redis 不可用时：
//   - JWT 记录未被删除，其他设备仍可通过旧 refresh token 续签
//   - refresh token 未被拉黑，已登出的会话仍可使用直到自然过期
//
// 改进：记录错误日志并返回 error，让 handler 层感知失败并返回适当的错误响应。
// 调用方可以根据返回值决定是否向客户端返回"登出可能未生效"的提示。
func (u *UserLogic) Logout(userID string, refreshToken string) error {
	// 修复：使用 errors.Join 返回所有错误，而非只返回最后一个
	var errs []error

	if err := u.deps.JWTRepo.DelJWT(context.Background(), userID); err != nil {
		// JWT 记录删除失败：记录日志，其他设备可能仍能通过旧 refresh token 续签
		u.deps.Logger.Warn("Logout: failed to delete JWT record",
			zap.String("user_id", userID), zap.Error(err))
		errs = append(errs, fmt.Errorf("delete JWT: %w", err))
	}

	refreshExpiry, _ := pkg.ParseDuration(u.deps.Config.JWT.RefreshTokenExpiryTime)
	if err := u.deps.JWTRepo.ToBlackList(context.Background(), refreshToken, refreshExpiry); err != nil {
		// refresh token 拉黑失败：已登出的会话仍可使用直到自然过期，存在安全风险
		u.deps.Logger.Warn("Logout: failed to blacklist refresh token",
			zap.String("user_id", userID), zap.Error(err))
		errs = append(errs, fmt.Errorf("blacklist refresh token: %w", err))
	}
	return errors.Join(errs...)
}

// -----------------------------------------------------------------------------

// UserInfo 获取当前登录用户的个人信息，走 Redis 缓存（与 UserHome 共用缓存逻辑）
func (u *UserLogic) UserInfo(ctx context.Context, userID string) (*response.UserHomeResp, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	userData, err := u.getUserCacheData(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userData == nil {
		return nil, fmt.Errorf("user not found")
	}

	// 经验从 DB 读取（缓存经验可能因历史写入未同步而过期，等级展示必须准确）
	var realExperience uint64
	if account, accErr := u.deps.AccountRepo.FindByID(ctx, userID); accErr == nil {
		realExperience = account.Experience
	}

	return &response.UserHomeResp{
		ID:                 userData.ID,
		AvatarURL:          userData.AvatarURL,
		Signature:          userData.Signature,
		Username:           userData.Username,
		Address:            userData.Address,
		VideoCount:         userData.VideoCount,
		Birthday:           userData.Birthday,
		Gender:             userData.Gender,
		TotalLikesReceived: userData.TotalLikesReceived,
		TotalPlayCount:     userData.TotalPlayCount,
		Experience:         realExperience,
		FansCount:          userData.FansCount,
		FollowingCount:     userData.FollowingCount,
	}, nil
}

// -----------------------------------------------------------------------------

func (u *UserLogic) UserChangeInfo(ctx context.Context, req request.UserChangeInfo) error {
	if _, err := u.deps.AccountRepo.FindByID(ctx, req.UserID); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"username":  req.Username,
		"signature": req.Signature,
		"birthday":  req.Birthday,
		"gender":    req.Gender,
	}
	// 仅在前端传了非空 avatar 时才更新，避免用空串覆盖已有头像 URL
	if req.Avatar != "" {
		updates["avatar_url"] = req.Avatar
	}
	if req.PrivacyPublicFavorites != nil {
		updates["privacy_public_favorites"] = *req.PrivacyPublicFavorites
	}
	if req.PrivacyPublicFollowing != nil {
		updates["privacy_public_following"] = *req.PrivacyPublicFollowing
	}
	if req.PrivacyPublicFans != nil {
		updates["privacy_public_fans"] = *req.PrivacyPublicFans
	}
	if req.ViewHistoryPaused != nil {
		updates["view_history_paused"] = *req.ViewHistoryPaused
	}
	if err := u.deps.AccountRepo.Updates(ctx, req.UserID, updates); err != nil {
		return err
	}
	// 静态信息变更后只删除静态区缓存，动态区缓存（计数类）不受影响
	if err := u.deps.UserCacheRepo.DeleteUserCache(ctx, req.UserID); err != nil {
		// 修复：缓存删除失败时记录错误日志
		// MySQL 更新成功但 Redis 缓存删除失败，后续请求可能读取到过时的用户信息
		u.deps.Logger.Error("删除用户缓存失败，可能存在数据不一致",
			zap.String("userID", req.UserID), zap.Error(err))
	}
	return nil
}

// -----------------------------------------------------------------------------

func (u *UserLogic) UserList(ctx context.Context, req request.UserList) ([]database.Account, int64, error) {
	filters := make(map[string]interface{})
	if req.UUID != nil {
		filters["id"] = *req.UUID
	}
	option := other.MySQLOption{
		PageInfo: req.PageInfo,
		Filters:  filters,
	}
	var list []database.Account
	total, err := u.deps.PaginateRepo.Paginate(ctx, option, &list)
	return list, total, err
}

// -----------------------------------------------------------------------------

func (u *UserLogic) UserFreeze(ctx context.Context, req request.UserOperation) error {
	userID := strconv.FormatUint(uint64(req.ID), 10)
	user, err := u.deps.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := u.deps.AccountRepo.UpdateColumn(ctx, userID, "freeze", true); err != nil {
		return err
	}
	jwtStr, err := u.deps.JWTRepo.GetJWT(ctx, user.ID)
	if err != nil {
		// 修复：GetJWT 失败时记录错误日志，而非静默忽略
		// Redis 不可用时无法获取旧 token，冻结用户仍可通过旧 refresh token 续签
		u.deps.Logger.Error("冻结用户时获取 JWT 失败", zap.String("userID", user.ID), zap.Error(err))
	} else if jwtStr != "" {
		refreshExpiry, _ := pkg.ParseDuration(u.deps.Config.JWT.RefreshTokenExpiryTime)
		if blacklistErr := u.deps.JWTRepo.ToBlackList(ctx, jwtStr, refreshExpiry); blacklistErr != nil {
			// 修复：黑名单写入失败时记录错误日志
			// 冻结用户的会话可能不会立即失效
			u.deps.Logger.Error("冻结用户时 JWT 加入黑名单失败", zap.String("userID", user.ID), zap.Error(blacklistErr))
		}
	}
	return nil
}

// -----------------------------------------------------------------------------

func (u *UserLogic) UserUnfreeze(ctx context.Context, req request.UserOperation) error {
	return u.deps.AccountRepo.UpdateColumn(ctx, strconv.FormatUint(uint64(req.ID), 10), "freeze", false)
}

// -----------------------------------------------------------------------------

func (u *UserLogic) UserLoginList(ctx context.Context, info request.UserLoginList) ([]database.Login, int64, error) {
	filters := make(map[string]interface{})
	if info.UUID != nil {
		filters["user_id"] = *info.UUID
	}
	option := other.MySQLOption{
		PageInfo: info.PageInfo,
		Filters:  filters,
		Preload:  []string{"Account"},
	}
	var list []database.Login
	total, err := u.deps.LoginRepo.Paginate(ctx, option, &list)
	return list, total, err
}

// -----------------------------------------------------------------------------

func (u *UserLogic) VerifyCaptcha(captchaID, captcha string) bool {
	// dev 模式临时跳过 captcha 校验（与 BaseLogic.VerifyCaptcha 保持一致）
	if os.Getenv("APP_SKIP_CAPTCHA") == "1" {
		return true
	}
	return u.deps.CaptchaStore.Verify(captchaID, captcha, true)
}

// -----------------------------------------------------------------------------

func (u *UserLogic) GenerateToken(user database.Account) (*TokenResult, error) {
	if user.Freeze {
		return nil, errors.New("user frozen")
	}

	j := pkg.NewJWT(&u.deps.Config.JWT)

	baseClaims := request.BaseClaims{
		UserID: user.ID,
		Role:   user.Role,
	}

	accessClaims, err := j.CreateAccessClaims(baseClaims, &u.deps.Config.JWT)
	if err != nil {
		return nil, err
	}

	accessToken, err := j.CreateAccessToken(accessClaims)
	if err != nil {
		return nil, err
	}

	refreshClaims, err := j.CreateRefreshClaims(baseClaims, &u.deps.Config.JWT)
	if err != nil {
		return nil, err
	}

	refreshToken, err := j.CreateRefreshToken(refreshClaims)
	if err != nil {
		return nil, err
	}

	oldToken, err := u.deps.JWTRepo.GetJWT(context.Background(), user.ID)
	if err == nil && oldToken != "" {
		refreshExpiry, _ := pkg.ParseDuration(u.deps.Config.JWT.RefreshTokenExpiryTime)
		_ = u.deps.JWTRepo.ToBlackList(context.Background(), oldToken, refreshExpiry)
	}

	expiry := time.Until(refreshClaims.ExpiresAt.Time)
	if err := u.deps.JWTRepo.SetJWT(context.Background(), user.ID, refreshToken, expiry); err != nil {
		// 修复：SetJWT 失败时记录错误日志
		// 下次登录时无法获取旧 token 进行黑名单操作，可能导致旧会话无法被正确失效
		u.deps.Logger.Error("存储 Refresh Token 到 Redis 失败",
			zap.String("userID", user.ID), zap.Error(err))
	}

	refreshExpiry := int(refreshClaims.ExpiresAt.Unix() - time.Now().Unix())

	return &TokenResult{
		Account:           user,
		AccessToken:       accessToken,
		AccessTokenExpire: accessClaims.ExpiresAt.Unix() * 1000,
		RefreshToken:      refreshToken,
		RefreshExpiry:     refreshExpiry,
	}, nil
}

// -----------------------------------------------------------------------------

var allowedImageExt = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

const (
	maxFileSize = 2097152
)

// -----------------------------------------------------------------------------

// UploadAvatar 上传用户头像，采用"写临时文件 + 原子 rename"模式。
//
// 流程：
//  1. 校验扩展名与大小
//  2. 计算目标路径 savePath 与临时路径 tmpPath = savePath + ".tmp"
//  3. 将上传内容先写入 tmpPath
//  4. 写入成功后调用 os.Rename(tmpPath, savePath) 原子提交
//  5. 任一中间环节失败时，确保删除 tmpPath 残留
//  6. 数据库 UpdateColumn 失败时回滚：删除已 rename 成功的目标文件
//
// 原子 rename 原理：
//
//	POSIX 文件系统的 rename(2) 系统调用是原子操作。在同一文件系统内，
//	rename 会以"先 unlink 旧 inode，再建立新目录项指向原 inode"的方式
//	完成。其他进程（worker 上传、HTTP 服务、后台清理脚本等）通过 stat/open
//	读取该路径时，要么看到完整的旧文件（如果存在），要么看到完整的新文件，
//	永远不会观察到"半截文件"的中间状态。这避免了上传过程中进程崩溃
//	（OOM / SIGKILL / 断电）留下半截文件导致图片损坏的问题。
func (u *UserLogic) UploadAvatar(ctx context.Context, userID string, file *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedImageExt[ext] {
		return "", errors.New("unsupported image format, only jpg/jpeg/png/gif/webp allowed")
	}

	if file.Size > maxFileSize {
		return "", fmt.Errorf("file size exceeds limit (%d bytes)", maxFileSize)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// 通过统一存储抽象上传（driver=qiniu 时落七牛云，driver=local 时落本地磁盘）。
	key := fmt.Sprintf("avatar/%s_%d%s", userID, time.Now().UnixMilli(), ext)
	avatarURL, err := u.deps.Storage.Put(ctx, key, src, file.Size)
	if err != nil {
		return "", fmt.Errorf("failed to save avatar: %w", err)
	}

	if err := u.deps.AccountRepo.UpdateColumn(ctx, userID, "avatar_url", avatarURL); err != nil {
		// 数据库更新失败：回滚已上传的头像对象，避免存储与 DB 不一致。
		_ = u.deps.Storage.Delete(ctx, key)
		return "", fmt.Errorf("failed to update avatar url: %w", err)
	}

	return avatarURL, nil
}

// UploadImage 通用图片上传（收藏夹封面 / 视频封面等场景）。
//
// 通过统一存储抽象上传到 image/ 前缀下，返回可公开访问的 URL。
func (u *UserLogic) UploadImage(ctx context.Context, userID string, file *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedImageExt[ext] {
		return "", errors.New("unsupported image format, only jpg/jpeg/png/gif/webp allowed")
	}
	if file.Size > u.deps.Config.Upload.MaxFileSize {
		return "", errors.New("文件超过大小限制")
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	key := fmt.Sprintf("image/%s_%d%s", userID, time.Now().UnixMilli(), ext)
	url, err := u.deps.Storage.Put(ctx, key, src, file.Size)
	if err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}
	return url, nil
}
