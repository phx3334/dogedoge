package initialize

import (
	"context"
	"fmt"
	"runtime"

	"fake_tiktok/internal/pkg/task"
	"fake_tiktok/internal/repository/mysql"
	redis_repo "fake_tiktok/internal/repository/redis"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// zapLogger：为 robfig/cron 适配 zap 日志接口
// ---------------------------------------------------------------------------

type zapLogger struct {
	logger *zap.Logger
}

func (l *zapLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, l.toFields(keysAndValues...)...)
}

func (l *zapLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.logger.Error(msg, append(l.toFields(keysAndValues...), zap.Error(err))...)
}

func (l *zapLogger) toFields(keysAndValues ...interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			fields = append(fields, zap.Any(key, keysAndValues[i+1]))
		}
	}
	return fields
}

// ---------------------------------------------------------------------------
// Cron 初始化与任务注册
// ---------------------------------------------------------------------------

// InitCron 创建支持秒级精度的 Cron 调度器实例。
// 使用 zap 作为日志后端，内置 panic 恢复链。
func InitCron(logger *zap.Logger) *cron.Cron {
	zl := &zapLogger{logger: logger}
	return cron.New(cron.WithLogger(zl), cron.WithChain(
		cron.Recover(cron.DefaultLogger), // 默认 panic 恢复
	), cron.WithSeconds()) // 启用秒级 cron 表达式
}

// RecoverCronJob 为单个 cron job 提供 panic 恢复包装器。
// 捕获 job 执行中的 panic，记录完整堆栈但不影响其他任务和主进程。
// 用法: c.AddJob(spec, RecoverCronJob(logger)(cron.FuncJob(fn)))
func RecoverCronJob(logger *zap.Logger) cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(func() {
			defer func() {
				if r := recover(); r != nil {
					buf := make([]byte, 64<<10) // 64KB 堆栈缓冲
					buf = buf[:runtime.Stack(buf, false)]
					logger.Error("cron job panic",
						zap.Any("recover", r),
						zap.String("stack", string(buf)),
					)
				}
			}()
			j.Run()
		})
	}
}

// CronJobDef 表示一个待注册的 cron 任务。
//
// 字段说明：
//   - Spec：cron 表达式或预定义周期，如 "@every 1m" / "@every 24h"
//   - Job：实际任务实现（FuncJob 包装的闭包）
//
// 设计原因（Task 5 拆分）：
//   - 旧版 StartCronTasks 直接注册 + Start，API 和 worker 无法复用同一份注册逻辑
//   - 新版 BuildCronJobs 返回 (Spec, Job) 列表，让 worker 端可以：
//     1. 直接遍历注册
//     2. 在注册前对 RebuildZSet 单独做"分布式锁包装"
//     3. 共享同一份"任务定义"逻辑，避免漂移
type CronJobDef struct {
	Spec string   // cron 表达式，如 "@every 1m"
	Job  cron.Job // 任务体
}

// BuildCronJobs 构造本进程要运行的定时任务定义列表（Task 5 拆分点）。
//
// 当前注册的任务：
//   - RebuildZSet      ：@every 1m   视频热度 ZSet 重建（Redis Pipeline 写）
//
// 注意：
//   - 本函数只构造"任务定义"，不调用 c.AddJob / c.Start
//   - 调用方负责遍历列表 + 注册 + 启动调度器
//   - 任务体内部用 context.Background()；不接收外部 ctx 是因为：
//     cron 任务的生命周期与调度器一致，无法对应到单一业务请求
//   - 这里不集成 Redis 分布式锁：是否需要锁取决于"是否多 worker 部署"，
//     由调用方（如 worker main）在拿到 list 后再单独包装 RebuildZSet
func BuildCronJobs(app *App) []CronJobDef {
	// 1. 构造 task 依赖：RedisClient、VideoRankingTask
	//
	// 修复说明（避免创建重复 PublishBuffer）：
	//   - 旧版调用 repository.NewRepos()，内部会创建 PublishBuffer + drainLoop
	//   - 但 cron 任务只需要 VideoRepo / RankingRepo / ClientRepo / AccountRepo，
	//     不需要 PlayCountPublisher（播放量发布）和 PublishBuffer
	//   - 如果 API 进程同时调用 InitRouter（创建一个 Repos）和 BuildCronJobs
	//     （又创建一个 Repos），就会有两个 PublishBuffer + 两个 drainLoop 竞争
	//     同一个 Connection，造成资源浪费和潜在的消息重复发送
	//   - 新版只构造 cron 任务实际需要的 repository，不创建多余的 PublishBuffer
	redisClient := redis_repo.NewRedisClient(app.Redis.Client, app.Redis.KeyPrefix)

	// 只构造 cron 任务需要的 repository，不调用 NewRepos 避免创建 PublishBuffer
	videoRepo := mysql.NewVideoRepo(app.DB)
	accountRepo := mysql.NewAccountRepo(app.DB)

	rankingTask := task.NewVideoRankingTask(
		videoRepo,
		redisClient, // RankingRepository 由 RedisClient 实现
		redisClient, // ClientRepository 由 RedisClient 实现
		accountRepo,
		app.DB,
		redisClient,
		app.Logger,
	)

	// 2. 构造 FuncJob
	//    闭包内捕获 rankingTask + app.Logger；执行失败仅记 Error 日志，
	//    不影响下一次调度（cron 调度器本身不知道任务成功/失败）
	//
	//    **Task 5 关键变更**：RebuildZSet 改用 RebuildZSetWithLock
	//    - 多 worker 部署下，通过 Redis 分布式锁保证互斥
	//    - 单 API + 单 worker 部署下，锁永远是本进程自己获得，行为一致
	//    - 抢不到锁（ErrLockHeld）时 RebuildZSetWithLock 内部直接 return nil，
	//      对调用方透明
	rebuildZSetJob := cron.FuncJob(func() {
		ctx := context.Background()
		if err := rankingTask.RebuildZSetWithLock(ctx); err != nil {
			app.Logger.Error("视频热度 ZSet 重建失败", zap.Error(err))
		}
	})

	// 3. 返回 (Spec, Job) 列表：调用方遍历注册即可
	return []CronJobDef{
		{Spec: "@every 1m", Job: rebuildZSetJob},
	}
}

// StartCronTasks 注册并启动所有定时任务（**保留向后兼容**）。
//
// 这是 BuildCronJobs + AddJob + Start 的便捷封装，供只需要"开箱即用"
// 的调用方使用（如旧的 API 进程在改造前曾直接调用）。
//
// 当前注册的任务（与 BuildCronJobs 一致）：
//   - 视频热度 ZSet 重建（@every 1m）
//
// 行为约定：
//   - 每个 job 都用 RecoverCronJob 包装：单个 job panic 不影响 cron 调度器
//   - 注册失败立即 return（不调用 c.Start）—— 启动失败要让调用方感知
//   - 成功注册后 c.Start() 启动后台调度
//
// 注意：worker 端在 Task 5 后不再调用本函数，而是直接遍历 BuildCronJobs
// 返回的列表注册，以便对 RebuildZSet 单独做"分布式锁包装"（详见 worker main）。
func StartCronTasks(c *cron.Cron, app *App) {
	for _, def := range BuildCronJobs(app) {
		if _, err := c.AddJob(def.Spec, RecoverCronJob(app.Logger)(def.Job)); err != nil {
			app.Logger.Error("注册 cron 任务失败",
				zap.String("spec", def.Spec),
				zap.Error(err),
			)
			return
		}
	}

	c.Start()
	app.Logger.Info("Cron 定时任务已启动（热度 ZSet 每 1 分钟重建，静态缓存每 24 小时同步）")
}
