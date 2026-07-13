// API 进程入口。
//
// 生命周期：
//  1. 加载配置
//  2. 初始化全局 App（DB / Redis / RabbitMQ / ES / Logger）
//  3. 根据 cfg.RunCron 决定是否创建 cron 调度器（**Task 5**：API 默认 false）
//  4. 用 signal.NotifyContext 构造根 ctx（监听 SIGINT/SIGTERM）
//  5. 把 ctx 交给 core.RunServer，由它负责"启动服务 + 优雅停机"
//
// 关键点：
//   - 不在 main 里 `defer app.Close()`：关闭顺序由 RunServer 内部
//     严格控制（srv.Shutdown → cron.Stop → app.Close），必须在收到
//     信号、停止接收新请求之后才能关闭资源；defer 走的总是最后才
//     执行，会破坏顺序
//   - 进程退出码：RunServer 返回非 nil error 时用 os.Exit(1) 体现
//     异常退出；返回 nil（即便触发了优雅停机）也用 0，符合容器
//     编排系统对"正常退出"的判定
//
// Task 5 变更：
//   - cron 不再无条件启动；通过 cfg.RunCron 控制
//   - API 默认不启动 cron（YAML 里 run_cron: false）
//   - 启用时仍走 initialize.StartCronTasks；不启用时传 nil 给 RunServer
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"fake_tiktok/internal/config"
	"fake_tiktok/internal/core"
	"fake_tiktok/internal/initialize"
)

func main() {
	// 1. 加载配置（YAML + 环境变量）
	cfg, err := config.LoadConfig()
	if err != nil {
		// 启动期连配置都加载不到时 zap.L() 还没初始化，使用 stderr
		// 这里仍尽量走 zap：调用 InitZap 之后才有意义，但配置加载
		// 阶段不需要 logger——直接 fmt 也可。此处保留原行为避免改动面
		zap.L().Error("加载配置失败", zap.Error(err))
		os.Exit(1)
	}

	// 2. 初始化全局 App：DB / Redis / RabbitMQ / ES / Logger
	app := initialize.NewApp(cfg)

	// 3. cron 启动控制（**Task 5**）：
	//   - 默认 cfg.RunCron == false：API 进程不注册任何 cron 任务
	//   - 启用时构造 cron 实例 + 注册任务；不启用时传 nil 给 RunServer
	//   - cron 内部以 goroutine 运行；启停由 RunServer 控制（nil 时跳过）
	var c *cron.Cron
	if cfg.RunCron {
		c = initialize.InitCron(app.Logger)
		initialize.StartCronTasks(c, app)
	} else {
		app.Logger.Info("cron disabled in API process (set APP_RUN_CRON=true to enable)")
	}

	// 4. 信号处理：把 SIGINT/SIGTERM 桥接到 context 取消
	//    - SIGINT  ：Ctrl+C / docker stop（默认 SIGTERM）
	//    - SIGTERM ：k8s 滚动更新、systemd 停止
	//    收到任一信号后 ctx 自动 Done，RunServer 即可感知
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 4.5 启动 RabbitMQ 重连循环（**Task 3 关键**）：
	//   - 必须在 InitRouter 之前启动，因为 router 内部 NewRepos 会构造
	//     PublishBuffer 并启动 drainLoop，drainLoop 会读 p.conn.runCtx
	//   - 必须 go 异步，否则会阻塞 main
	//   - 用主 ctx 派生，信号触发时 Run 协程自动退出
	go app.RabbitMQConn.Run(ctx)

	// 5. 启动服务 + 优雅停机。RunServer 返回的 error 含义：
	//   - nil：正常退出（信号触发的优雅停机 或 RunServer 内部清理完）
	//   - 非 nil：服务启动失败 / 监听失败 / 资源关闭异常
	if err := core.RunServer(ctx, &cfg.Server, app, c); err != nil {
		app.Logger.Error("server exited with error", zap.Error(err))
		os.Exit(1)
	}
}
