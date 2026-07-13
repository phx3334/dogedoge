// Package initialize 内的 app.go：聚合应用启动期需要的所有外部资源。
//
// 关键变更（Task 3）：
//   - App.RabbitMQ (*initialize.RabbitMQ) → App.RabbitMQConn (*rabbitmq.Connection)
//   - 不在 NewApp 中启动 conn.Run（让调用方传 ctx，融入 signal.NotifyContext 体系）
//   - Close() 中使用 5s 超时兜底：rabbitmq 重连循环可能在 broker 不可用时阻塞 Close
package initialize

import (
	"context"
	"fake_tiktok/internal/config"
	es_repo "fake_tiktok/internal/repository/es"
	rabbitmq "fake_tiktok/internal/repository/rabbitmq"

	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// App 聚合了所有进程级外部资源。
//
// 每个字段都为单例：DB / Redis / RabbitMQ / ES 全进程共享同一份连接池 / 客户端。
// Logger 是 zap 全局 logger，复制指针即可使用。
type App struct {
	Config *config.Config
	DB     *gorm.DB
	Redis  *RedisConn
	// RabbitMQConn Task 3 改造后改为持有 *rabbitmq.Connection 抽象。
	// 调用方负责：go RabbitMQConn.Run(ctx)
	RabbitMQConn *rabbitmq.Connection
	ESClient     *elasticsearch.TypedClient
	Logger       *zap.Logger
}

// NewApp 按顺序初始化所有外部资源。
//
// 初始化顺序（不要随意调整）：
//  1. Logger：先建好 logger，后续初始化失败都能打印
//  2. DB：业务强依赖
//  3. AutoMigrate：在 DB 之上
//  4. Redis：缓存层
//  5. RabbitMQConn：MQ 抽象（**不**在此处 Run；留给 main 启动）
//  6. ES + Index：搜索层
//
// 注意：RabbitMQConn 没有立即建连——Connection.Run 才是首次 dial 入口。
// 这样启动时 broker 不可用也不会让进程退出。
func NewApp(cfg *config.Config) *App {
	var err error
	app := &App{
		Config: cfg,
	}

	app.Logger = InitZap(&cfg.Zap)

	app.DB = InitGorm(cfg)

	if cfg.Database.AutoMigrate {
		if err := AutoMigrate(app.DB, app.Logger); err != nil {
			app.Logger.Error("数据库迁移失败", zap.Error(err))
		}
	}

	app.Redis, err = ConnectRedis(&cfg.Redis)
	if err != nil {
		app.Logger.Error("Redis初始化失败", zap.Error(err))
	}

	// Task 3 改造：返回 Connection 抽象；调用方负责 go conn.Run(ctx)
	app.RabbitMQConn = NewRabbitMQConn(cfg, app.Logger)

	app.ESClient = ConnectEs(cfg)
	esRepo := es_repo.NewSearchIndexRepo(app.ESClient)
	InitEsIndex(esRepo)

	// 全量回填已发布视频到 ES 索引（幂等，每次启动执行一次）
	// 保证存量视频可被搜索到；新视频由 worker 转码完成后实时索引
	if app.ESClient != nil {
		videoSearchRepo := es_repo.NewVideoSearchRepo(app.ESClient, app.Logger)
		BackfillEsVideoIndex(app.DB, videoSearchRepo, app.Logger)
	}

	return app
}

// Close 按顺序释放所有外部资源。
//
// 释放顺序（与初始化顺序相反的"软规则"）：
//  1. DB / Redis / ES：标准 Close
//  2. RabbitMQConn：5s 超时兜底——Connection.Close 内部会先 cancel 重连循环，
//     但若 broker 长时间 hang 住 socket，重连循环可能阻塞在 dial 上不退出
//     （虽然 amqp091-go 有内置连接超时，但保守起见仍加超时）
func (app *App) Close() error {
	var lastErr error

	if app.DB != nil {
		sqlDB, err := app.DB.DB()
		if err != nil {
			app.Logger.Error("获取数据库连接失败", zap.Error(err))
			lastErr = err
		} else {
			if err := sqlDB.Close(); err != nil {
				app.Logger.Error("关闭数据库连接失败", zap.Error(err))
				lastErr = err
			}
		}
	}

	if app.Redis != nil {
		if err := app.Redis.Client.Close(); err != nil {
			app.Logger.Error("关闭 Redis 连接失败", zap.Error(err))
			lastErr = err
		}
	}

	// RabbitMQ 关闭走 5s 超时：Connection.Close 内部是同步等 Run 退出，
	// 万一 broker 状态异常导致 Run 卡住，5s 后强制放行（Connection 内部
	// 还有 done channel 兜底，5s 后即便没收到 done 也不会泄漏 fd 太久）
	if app.RabbitMQConn != nil {
		closed := make(chan struct{})
		go func() {
			app.RabbitMQConn.Close()
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			app.Logger.Error("rabbitmq close timeout")
		}
	}

	if app.ESClient != nil {
		if err := app.ESClient.Close(context.Background()); err != nil {
			app.Logger.Error("关闭 Elasticsearch 连接失败", zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}
