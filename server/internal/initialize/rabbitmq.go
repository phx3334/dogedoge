// Package initialize 内的 rabbitmq.go：构造 rabbitmq.Connection 抽象。
//
// **BREAKING 变更（Task 3）**：
//   - 旧版：直接 amqp.Dial 并暴露 Conn / Channel 字段
//   - 新版：返回 *rabbitmq.Connection，由调用方负责 go conn.Run(ctx)
//
// 调用方约定：
//   - 在合适的 ctx（main 的 signal ctx）下：go conn.Run(ctx)
//   - 通过 rabbitmq.NewPublishBuffer 包装后传给 Repos
package initialize

import (
	"fake_tiktok/internal/config"
	"fake_tiktok/internal/repository/rabbitmq"

	"go.uber.org/zap"
)

// NewRabbitMQConn 创建一个 *rabbitmq.Connection 抽象。
//
// 与旧实现的差异：
//   - 不再立即 amqp.Dial；Connection.Run 才会发起第一次连接
//   - 启动时 RabbitMQ 不可用不会阻塞进程（重连循环异步恢复）
//   - 调用方必须自行：go conn.Run(ctx)
//
// 参数：
//   - cfg：应用总配置，函数内部取 &cfg.RabbitMQ
//   - logger：用于记录连接建立/断开/重连等待
func NewRabbitMQConn(cfg *config.Config, logger *zap.Logger) *rabbitmq.Connection {
	return rabbitmq.NewConnection(&cfg.RabbitMQ, logger)
}
