//go:build windows
// +build windows

// Windows 平台下的 Server 实现。
//
// 历史上该文件使用 endless 库通过 fork 子进程实现"伪热重启"。
// 但 endless 跨平台行为差异大、Windows 下不支持 fork，与本项目
// "统一优雅停机"的目标冲突。Task 2 之后 Windows 与 Linux 共用
// 标准库 net/http 的 *http.Server，通过 Server 接口对外暴露。
package core

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// httpServer 是对 *http.Server 的薄包装，作用是：
//  1. 满足本包定义的 Server 接口（Start / Shutdown）
//  2. 把 platform-specific 的细节（监听、关闭）封装在一个类型里，
//     上层 RunServer 无需关心是 Linux 还是 Windows
//
// 使用结构体嵌入（struct embedding）复用 *http.Server 的全部方法，
// 避免逐个转发造成的代码冗余。
type httpServer struct {
	*http.Server
}

// Start 启动 HTTP 监听。这是一个阻塞调用：只要服务没出错就一直运行。
//
// 实现细节：直接调用底层 *http.Server.ListenAndServe()。正常情况下
// 返回 http.ErrServerClosed（在 Shutdown 之后），调用方可以忽略。
//
// 注意：这里不需要捕获 panic——RunServer 入口处由 recover / 日志
// 体系负责；本方法只关心"启动是否成功"。
func (s *httpServer) Start() error {
	return s.ListenAndServe()
}

// Shutdown 优雅关闭 HTTP 服务。
//
// 行为：*http.Server.Shutdown 会先停止接收新连接，然后等待已建立
// 的连接上所有 in-flight 请求处理完成；当 ctx 超时或被取消时，
// 返回 context.DeadlineExceeded。RunServer 会捕获并记录该错误。
//
// 参数 ctx 来自 RunServer 内的 30s 超时控制，跨平台语义一致。
func (s *httpServer) Shutdown(ctx context.Context) error {
	return s.Server.Shutdown(ctx)
}

// InitServer 构造 Windows 平台下的 Server 实例。
//
// 超时设置（10s 读 / 10s 写）属于保守默认：
//   - 读超时覆盖整个请求头 + body，避免慢客户端长期占用连接
//   - 写超时从读第一个 body 字节开始计时，给正常业务（上传、视频代理）足够空间
//   - MaxHeaderBytes = 1MB 防止恶意大 header 撑爆内存
//
// 返回值类型为 Server 接口，使得 RunServer 内的处理逻辑对平台无感。
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
