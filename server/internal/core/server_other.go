//go:build !windows
// +build !windows

// 非 Windows 平台（Linux / macOS / BSD 等）下的 Server 实现。
//
// 早期版本依赖 github.com/fvbock/endless 实现信号处理 + "热重启"，
// 但 endless 通过 fork 子进程实现重启，跨平台行为差异大、对容器
// 信号不友好。Task 2 改造后，本项目改用标准库 net/http + 自管
// signal.NotifyContext 路径：所有平台（包括 Windows）走同一份
// httpServer，endless 依赖已从 go.mod 移除。
package core

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// httpServer 是对 *http.Server 的薄包装，作用：
//  1. 满足本包定义的 Server 接口（Start / Shutdown）
//  2. 与 Windows 端实现完全一致，确保 RunServer 在所有平台上
//     看到的是同一个行为模型，便于回归测试
//
// 嵌入 *http.Server 后，httpServer 直接"继承"了 Server 字段与所有
// 方法；这里只额外暴露 Start / Shutdown 两个对外契约。
type httpServer struct {
	*http.Server
}

// Start 启动 HTTP 监听，阻塞直到服务退出。
//
// 调用 ListenAndServe：返回 nil 表示异常退出（监听失败等），
// 返回 http.ErrServerClosed 是被 Shutdown 优雅关闭后的正常路径。
//
// 历史上 endless 的 ListenAndServe 内部捕获 SIGUSR1/SIGHUP 实现
// "热重启"，现在我们不再需要这个能力——容器场景下"无停机发布"由
// 编排层（k8s rollingUpdate 等）负责，进程内自管容易引入状态不一致。
func (s *httpServer) Start() error {
	return s.ListenAndServe()
}

// Shutdown 优雅关闭 HTTP 服务。
//
// 语义与 Windows 端完全一致：
//   - 停止接受新连接
//   - 等待 in-flight 请求完成
//   - ctx 超时后强制中断
//
// RunServer 中传入的 ctx 携带 30s 硬上限，避免卡死的请求拖死停机。
func (s *httpServer) Shutdown(ctx context.Context) error {
	return s.Server.Shutdown(ctx)
}

// InitServer 构造非 Windows 平台下的 Server 实例。
//
// 参数与 Windows 端保持一致（ReadTimeout / WriteTimeout / MaxHeaderBytes），
// 这样在两个平台上跑出来的连接管理行为完全一致——这是把 endless
// 替换成 net/http 的关键收益之一。
//
// 返回 Server 接口，调用方在编译期就与具体平台解耦。
func InitServer(addr string, Router *gin.Engine) Server {
	return &httpServer{
		Server: &http.Server{
			Addr:           addr,
			Handler:        Router,
			ReadTimeout:    3600 * time.Second,
			WriteTimeout:   3600 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
	}
}
