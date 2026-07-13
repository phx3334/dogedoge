// Package core 提供跨平台统一的 HTTP 服务抽象与生命周期管理。
//
// 本文件定义核心的 Server 接口与 RunServer 入口函数。
// 与平台相关的具体实现（Windows / 类 Unix）通过 build tag 拆分到
// server_win.go 与 server_other.go 中，但对外只暴露 Server 一个抽象。
package core

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"fake_tiktok/internal/config"
	"fake_tiktok/internal/initialize"
)

// Server 抽象了"可启动 + 可关闭"的 HTTP 服务。
//
// 关键点：
//   - Start() 是阻塞调用，监听失败或服务退出时返回 error
//   - Shutdown(ctx) 由调用方在收到关闭信号后主动触发，框架会等待
//     已建立的连接完成处理或 ctx 超时（实现由底层 *http.Server 保证）
//   - 平台无关：Windows / Linux 都使用同一份接口，避免 endless 那类
//     依赖 fork 的库带来的跨平台行为差异
type Server interface {
	Start() error
	Shutdown(ctx context.Context) error
}

// RunServer 是 API 进程的主入口。
//
// 参数说明：
//   - ctx：根上下文，由 main 用 signal.NotifyContext 包装。
//     一旦收到 SIGINT/SIGTERM，ctx 自动取消，本函数据此触发优雅停机。
//   - cfg：服务监听配置（host / port）
//   - app：全局应用对象（DB / Redis / RabbitMQ / Logger / ES）
//   - cron：后台定时任务调度器，由 main 在 RunServer 之前 Start
//
// 关闭顺序（重要）：
//  1. 先 srv.Shutdown：停止接收新请求，等待在途请求完成
//  2. 再 cron.Stop：避免停机期间 cron 触发新的写操作
//  3. 最后 app.Close：释放 DB / Redis / MQ / ES 连接
//
// 顺序解释：cron 可能调用 MySQL/Redis，所以必须先停 cron 再关连接；
// srv.Shutdown 是最慢的（要等请求），放最前面能与其他清理并行进行。
// 即便 Shutdown 内部失败，也会强制继续执行后续清理，保证进程可退出。
//
// cron 参数（**Task 5 改动**）：
//   - 允许为 nil：当 cfg.RunCron == false（API 默认值）时，main 不构造
//     cron 调度器，直接传 nil 给本函数
//   - nil 语义：跳过 cron.Stop 调用；其他清理逻辑（srv.Shutdown / app.Close）正常执行
//   - 避免出现"为了停一个不存在的 cron 而 panic"的问题
func RunServer(ctx context.Context, cfg *config.ServerConfig, app *initialize.App, cron *cron.Cron) error {
	// 1. 构造监听地址并初始化路由
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	router := initialize.InitRouter(app)

	// 2. 构造平台相关的 Server 实现（Windows / 其他走 build tag 分支）
	srv := InitServer(addr, router)

	app.Logger.Info("server starting on " + addr)

	// 3. 在独立 goroutine 中启动服务，避免阻塞 select
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// 4. 等待以下任一事件：
	//   - 服务自身退出（监听失败 / panic）→ 把 err 透传给调用方
	//   - ctx 被取消（信号）→ 走优雅停机流程
	select {
	case err := <-errCh:
		// 服务异常退出：尝试继续清理资源
		if err != nil {
			app.Logger.Error("server start failed", zap.Error(err))
		}
		// 即便 Start 返回，也尽量把 cron 和资源关掉
		// （**Task 5**）cron 可能为 nil——API 默认不启用 cron，main 直接传 nil
		if cron != nil {
			cron.Stop()
		}
		if app != nil {
			_ = app.Close()
		}
		return err

	case <-ctx.Done():
		app.Logger.Info("shutdown signal received, draining in-flight requests...")
	}

	// 5. 优雅停机：30s 硬上限，超时强制继续
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// 不直接 return：保证后续清理仍会执行
		app.Logger.Error("server shutdown error", zap.Error(err))
	} else {
		app.Logger.Info("http server stopped")
	}

	// 6. 停止 cron 调度器
	//    （**Task 5**）cron 为 nil 时跳过：API 默认不启用 cron，无需清理
	if cron != nil {
		app.Logger.Info("stopping cron...")
		cron.Stop()
		app.Logger.Info("cron stopped")
	}

	// 7. 释放下游资源（DB / Redis / MQ / ES）
	if app != nil {
		app.Logger.Info("closing app resources...")
		if err := app.Close(); err != nil {
			app.Logger.Error("app close error", zap.Error(err))
		}
	}

	app.Logger.Info("graceful shutdown complete")
	return nil
}
