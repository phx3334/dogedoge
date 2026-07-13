// Worker 进程入口：消费 RabbitMQ 中的播放量增量消息，定期批量写回 MySQL。
//
// 生命周期：
//  1. 加载配置
//  2. 初始化全局 App
//  3. 启动 RabbitMQ 重连循环（go app.RabbitMQConn.Run(ctx)）
//  4. 用 signal.NotifyContext 构造根 ctx（监听 SIGINT/SIGTERM）
//  5. **Task 5**：若 cfg.RunCron=true，启动 cron 调度器（worker 默认开启）
//  6. 启动 consumeLoop（外层 WaitReady → 内层 Consume 循环）+ flush 协程
//  7. 主线程阻塞等待信号；收到信号后做最后一次 flush + 关闭资源
//
// 关键改造（Task 3 + Task 5）：
//   - 不再使用 app.RabbitMQ.Channel 字段
//   - 引入 *rabbitmq.Connection 抽象，支持自动重连
//   - consumeLoop 拆为独立函数；channel 关闭时外层循环自动重订阅
//   - **Task 5**：cron 启动由 cfg.RunCron 控制；worker 端默认开启
//   - **Task 5**：RebuildZSet 通过 Redis 分布式锁保证多 worker 互斥
//
// 关闭顺序：
//  1. 信号触发 ctx.Done()：consumeLoop 与 flush 协程各自退出
//  2. flush 协程退出前主动调用一次 flush()，把内存里的 pending 全部落库
//  3. 主线程在 ctx.Done() 后再等一小段时间（5s shutdownCtx 兜底）
//  4. 调 app.Close() 释放连接
//
// 为什么不用 defer app.Close()：
//
//	defer 在 main 返回时执行；如果 main 在收到信号后立即 return，
//	consumeLoop/flush 协程可能还在跑，会和资源关闭形成竞态。
//	所以这里改成显式调用 app.Close()，与 5s 超时上下文配合。
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"fake_tiktok/internal/config"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/initialize"
	"fake_tiktok/internal/pkg/storage"
	es_repo "fake_tiktok/internal/repository/es"
	"fake_tiktok/internal/repository/interfaces"
	"fake_tiktok/internal/repository/rabbitmq"
	redis_repo "fake_tiktok/internal/repository/redis"
)

func main() {
	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		zap.L().Error("加载配置失败", zap.Error(err))
		os.Exit(1)
	}

	// 2. 初始化全局 App
	app := initialize.NewApp(cfg)

	// 3. 信号处理：SIGINT / SIGTERM 任一到达即触发优雅停机
	//    把 ctx 作为根 ctx 传给 RabbitMQConn.Run 和 consumeLoop；
	//    信号触发时所有 goroutine 通过 ctx 链路统一退出
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 4. 启动 RabbitMQ 重连循环（**关键**）：
	//    - 必须 go 异步，否则会阻塞 main
	//    - 必须用 main 的 ctx 派生，信号触发时 Run 协程自动退出
	//    - Run 内部会处理 dial 失败、NotifyClose 重连等所有细节
	go app.RabbitMQConn.Run(ctx)

	// 4.5 **Task 5**：启动 cron 调度器
	//   - worker 端默认 cfg.RunCron = true（docker-compose 中通过
	//     环境变量 APP_RUN_CRON: "true" 显式覆盖 config.docker.yaml）
	//   - 通过 BuildCronJobs 拿到 (Spec, Job) 列表，逐个注册到 cron
	//   - cron 内部以 goroutine 运行；信号触发时由 cron.Stop 统一停
	var c *cron.Cron
	if cfg.RunCron {
		c = initialize.InitCron(zap.L())
		for _, def := range initialize.BuildCronJobs(app) {
			if _, err := c.AddJob(def.Spec, initialize.RecoverCronJob(zap.L())(def.Job)); err != nil {
				zap.L().Error("add cron job failed",
					zap.String("spec", def.Spec),
					zap.Error(err),
				)
			}
		}
		c.Start()
		zap.L().Info("cron started in worker process")
	} else {
		zap.L().Info("cron disabled in worker process (set APP_RUN_CRON=true to enable)")
	}

	// 5. 内存批量聚合：消息先入 pending map，flush 时再批量 UPDATE
	//    当前是顺序写 MySQL（每条 SQL 一次 round-trip），Task 7 编译验证后保留
	pending := make(map[uint]int64)
	var mu sync.Mutex

	userPending := make(map[string]int64)
	var userMu sync.Mutex

	// flush 将内存中累积的播放量增量批量写入 MySQL。
	//
	// 数据安全保证：
	//   - 先复制快照再释放锁，避免 UPDATE 持锁时间过长阻塞消费
	//   - UPDATE 成功后才从 pending 中减去对应增量
	//   - UPDATE 失败时 pending 保留原值，下次 flush 时重试
	//   - 这样即使 MySQL 短暂不可用，增量数据也不会丢失
	//
	// 关键设计：复制 batch 后不清空 pending
	//   - 旧版在复制后立即 delete(pending, k)，UPDATE 失败时数据丢失
	//   - 新版保留 pending 原值，只减去已成功写入 MySQL 的增量
	//   - 失败的条目保留在 pending 中不动，下次 flush 自然重试
	flush := func() {
		mu.Lock()
		if len(pending) == 0 {
			mu.Unlock()
			return
		}
		// 复制一份快照再释放锁：UPDATE 可能耗时较长，
		// 持锁期间会阻塞 consumeLoop 中的消息累加操作
		batch := make(map[uint]int64, len(pending))
		for k, v := range pending {
			batch[k] = v
		}
		mu.Unlock()

		// 逐条 UPDATE：记录成功的视频 ID，后续从 pending 中减去
		// 使用带 5 秒超时的 context，防止 MySQL 慢查询阻塞 flush 协程
		// 导致无法响应停机信号，worker 进程无法优雅关闭
		var succeededIDs []uint
		for videoID, increment := range batch {
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
			result := app.DB.WithContext(updateCtx).Model(&struct{}{}).Table("videos").
				Where("id = ?", videoID).
				UpdateColumn("play_count", gorm.Expr("play_count + ?", increment))
			updateCancel()
			if result.Error != nil {
				zap.L().Error("更新播放量失败",
					zap.Uint("video_id", videoID),
					zap.Int64("increment", increment),
					zap.Error(result.Error))
				// UPDATE 失败：pending 中保留原值，下次 flush 重试
			} else {
				succeededIDs = append(succeededIDs, videoID)
			}
		}

		// 从 pending 中减去已成功写入 MySQL 的增量
		// 注意：flush 期间 consumeLoop 可能往 pending 中累加了新的增量，
		// 所以 pending[videoID] 可能已经 > batch[videoID]
		mu.Lock()
		for _, videoID := range succeededIDs {
			increment := batch[videoID]
			if pendingVal, ok := pending[videoID]; ok {
				if pendingVal <= increment {
					// pending 值 ≤ 本次写入的增量：直接删除
					// （flush 期间没有新增量，或新增量恰好使值相等）
					delete(pending, videoID)
				} else {
					// pending 值 > 本次写入的增量：减去已成功写入的部分
					// （flush 期间有新的增量累加进来）
					pending[videoID] = pendingVal - increment
				}
			}
		}
		mu.Unlock()
	}

	userFlush := func() {
		userMu.Lock()
		if len(userPending) == 0 {
			userMu.Unlock()
			return
		}
		batch := make(map[string]int64, len(userPending))
		for k, v := range userPending {
			batch[k] = v
		}
		userMu.Unlock()

		var succeededIDs []string
		for userID, increment := range batch {
			// 使用带 5 秒超时的 context，防止 MySQL 慢查询阻塞 userFlush 协程
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
			result := app.DB.WithContext(updateCtx).Model(&struct{}{}).Table("accounts").
				Where("id = ?", userID).
				UpdateColumn("total_play_count", gorm.Expr("total_play_count + ?", increment))
			updateCancel()
			if result.Error != nil {
				zap.L().Error("更新用户总播放量失败",
					zap.String("user_id", userID),
					zap.Int64("increment", increment),
					zap.Error(result.Error))
			} else {
				succeededIDs = append(succeededIDs, userID)
			}
		}

		userMu.Lock()
		for _, userID := range succeededIDs {
			increment := batch[userID]
			if pendingVal, ok := userPending[userID]; ok {
				if pendingVal <= increment {
					delete(userPending, userID)
				} else {
					userPending[userID] = pendingVal - increment
				}
			}
		}
		userMu.Unlock()
	}

	// 6. 启动 flush 协程：5s 一次常规 flush，ctx 取消时做最后一次
	//    注意：最后一次 flush 不能省略——可能还有未落库的增量
	//
	// WaitGroup 跟踪后台协程生命周期：
	//   - 确保 app.Close() 在 flush 协程完成最后一次 flush 之后才执行
	//   - 避免 DB 连接在 flush 的 UPDATE 操作完成前被关闭导致数据丢失
	var wg sync.WaitGroup

	// ---- 点赞数增量聚合 ----
	// 与播放量增量模式一致：消息先入 pending map，flush 时批量 UPDATE
	// 延迟 3 秒聚合：短时间内多个用户点赞同一视频，合并为一次 UPDATE likes_count + N
	likePending := make(map[uint]int64)
	var likeMu sync.Mutex

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				likeFlush(app.DB, &likePending, &likeMu)
				return
			case <-ticker.C:
				likeFlush(app.DB, &likePending, &likeMu)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		likeConsumeLoop(ctx, app.RabbitMQConn, zap.L(), &likePending, &likeMu)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// 收到停机信号：做最后一次 flush，把内存里所有增量落库
				flush()
				return
			case <-ticker.C:
				flush()
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				userFlush()
				return
			case <-ticker.C:
				userFlush()
			}
		}
	}()

	// 7. 启动 consumeLoop：ctx 取消或 channel 关闭即退出
	//    外层循环负责"等连接 + 重新订阅"，内层循环负责"逐条处理消息"
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumeLoop(ctx, app.RabbitMQConn, app.DB, zap.L(), &pending, &mu)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		userConsumeLoop(ctx, app.RabbitMQConn, zap.L(), &userPending, &userMu)
	}()

	// 7.5 启动 transcode 消费者：消费 mini_bili_transcode 队列，调用 ffmpeg 转码
	//    与现有 consumeLoop 模式一致：外层 WaitReady + 重订阅 / 内层消息处理
	//    Storage 实例用于把转码后的 MP4 和封面 JPG 上传到对象存储或本地磁盘
	//    若 Storage 初始化失败（如 OSS 配置缺失），transcode 消费者跳过启动，
	//    其他消费者（播放量/点赞数）仍正常运行
	storageInstance, err := storage.NewStorage(&cfg.Storage, &cfg.Qiniu)
	if err != nil {
		zap.L().Error("failed to init storage for transcode worker", zap.Error(err))
	}

	// 构造 VideoSearchRepo 用于转码完成后将视频索引到 ES
	// app.ESClient 在 ES 不可用时为 nil，此时 videoSearchRepo 也为 nil，
	// transcodeConsumeLoop 内部会跳过 ES 索引步骤，不影响视频发布
	var videoSearchRepo interfaces.VideoSearchRepository
	if app.ESClient != nil {
		videoSearchRepo = es_repo.NewVideoSearchRepo(app.ESClient, zap.L())
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if storageInstance == nil {
			zap.L().Warn("storage not initialized, transcode consumer disabled")
			return
		}
		transcodeConsumeLoop(ctx, app.RabbitMQConn, app.DB, storageInstance, cfg.FFmpeg, cfg.Upload.TempUploadDir, videoSearchRepo, zap.L())
	}()

	zap.L().Info("Worker 已启动，正在消费播放量增量消息和用户播放量增量消息...")

	// 8. 阻塞等待信号；收到后主线程继续走关闭流程
	<-ctx.Done()
	zap.L().Info("worker shutdown signal received")

	// 8.5 **Task 5**：关闭 cron 调度器（仅当启用了）
	//    robfig/cron/v3 的 Stop() 返回一个 Context，在所有正在运行的 job 完成后取消
	//    使用 5 秒超时等待，避免 cron job 卡住导致 worker 无法退出
	if c != nil {
		zap.L().Info("stopping cron...")
		cronCtx := c.Stop()
		select {
		case <-cronCtx.Done():
			zap.L().Info("cron stopped, all jobs finished")
		case <-time.After(5 * time.Second):
			zap.L().Warn("cron stop timeout (5s), some jobs may still be running")
		}
	}

	// 8.6 等待 consumeLoop 和 flush 协程退出（带 10 秒超时）
	//     这是优雅停机的关键步骤：
	//     - consumeLoop 在 ctx.Done() 后会 nack 未处理消息并退出
	//     - flush 协程在 ctx.Done() 后会做最后一次 flush 再退出
	//     - 必须等它们都退出后才能关闭 DB/Redis/MQ 连接
	//     - 否则 flush 的 UPDATE 可能因 DB 连接已关闭而失败，导致数据丢失
	//
	//     超时兜底（10 秒）：
	//     - 正常情况：协程在 1~2s 内退出，waitCh 很快返回
	//     - 异常情况：consumeLoop 可能阻塞在 for range msgs（无消息时不会检查 ctx.Done）
	//       此时需要 app.Close() 关闭 RabbitMQ 连接才能解除阻塞
	//     - 超时后继续走 app.Close()，关闭连接后 goroutine 自然退出
	zap.L().Info("waiting for goroutines to exit...")
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()
	select {
	case <-waitCh:
		zap.L().Info("all goroutines exited")
	case <-time.After(10 * time.Second):
		zap.L().Warn("worker goroutines exit timeout (10s), forcing shutdown")
	}

	// 9. 关闭全局资源（DB / Redis / RabbitMQ / ES）
	//    app.Close() 内部对 RabbitMQ 连接关闭有 5s 超时兜底
	//    关闭 RabbitMQ 连接会使 consumeLoop 的 msgs channel 关闭，解除 goroutine 阻塞
	if err := app.Close(); err != nil {
		zap.L().Error("关闭应用资源失败", zap.Error(err))
	}
}

// consumeLoop 是 worker 端消费者的重订阅循环（Task 3 核心改动）。
//
// 外层循环：等连接就绪 → 注册消费者 → 阻塞到 channel 关闭 → 回到起点。
// 内层循环：从 msgs channel 读消息 → 解析 → 累加到 pending → ack。
//
// 为什么是双层循环：
//   - amqp091 的 Consume() 返回的 msgs channel 在连接断开时会自动关闭
//   - 连接断开后必须重新 Consume 才能继续接收消息
//   - 外层循环负责"重新建立消费"，内层循环负责"持续处理消息"
//
// 为什么每次都要重新 QueueDeclare：
//   - 严格来说不需要（durable 队列在 broker 端持久化）
//   - 但加入"重新声明"可以更早发现 broker 端队列被误删等异常
//   - 代价是每次重连多一次 RTT；当前为简化实现而保留
//
// 关于 ctx.Done()：
//   - 收到停机信号时，未 ack 的消息 nack(requeue=true) 让 broker 重新投递
//   - 这样多 worker 部署时未处理完的消息不会丢失
func consumeLoop(ctx context.Context, conn *rabbitmq.Connection, db *gorm.DB, logger *zap.Logger, pending *map[uint]int64, mu *sync.Mutex) {
	for {
		// 1. 阻塞等连接可用：MQ 启动 / 重连期间这里会短暂阻塞
		ch, err := conn.WaitReady(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// 主流程已经收信号：正常退出
				return
			}
			logger.Warn("wait ready failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		// 2. 声明队列（durable + non-auto-delete）
		//    即便 worker 端反复重启，broker 端队列不会丢
		if _, err := ch.QueueDeclare(redis_repo.PlayCountQueueName, true, false, false, false, nil); err != nil {
			logger.Warn("queue declare failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		// 3. Qos=10：未 ack 消息超过 10 条时 broker 停止推送
		//    避免 worker 端堆积 + 防止慢消费压垮内存
		if err := ch.Qos(10, 0, false); err != nil {
			logger.Warn("qos set failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		// 4. 注册消费者：autoAck=false 让我们能精确控制 ack 时机
		msgs, err := ch.Consume(redis_repo.PlayCountQueueName, "", false, false, false, false, nil)
		if err != nil {
			logger.Warn("consume register failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		logger.Info("consume registered, waiting for messages")

		// 5. 内层消息处理循环
		//    msgs channel 在以下情况会关闭：
		//      - Connection 断开（broker 端断开 socket）
		//      - ch.Cancel() 被调用
		//    此时 for range 退出，回到外层重新 WaitReady
		for msg := range msgs {
			// 停机信号优先：先 nack + requeue 让消息回到 broker，再退出
			select {
			case <-ctx.Done():
				_ = msg.Nack(false, true)
				return
			default:
			}

			var inc cache.PlayCountIncrementMsg
			if err := json.Unmarshal(msg.Body, &inc); err != nil {
				// 消息体损坏：nack(requeue=false) 让它进死信或被丢弃
				_ = msg.Nack(false, false)
				continue
			}

			mu.Lock()
			(*pending)[inc.VideoID] += inc.Increment
			mu.Unlock()

			_ = msg.Ack(false)
		}

		// 走到这里说明 msgs channel 关闭（broker 断开 / channel 被 cancel）
		// 回到外层 WaitReady 重新订阅
		logger.Warn("consume channel closed, will re-subscribe after reconnect")
	}
}

func userConsumeLoop(ctx context.Context, conn *rabbitmq.Connection, logger *zap.Logger, pending *map[string]int64, mu *sync.Mutex) {
	for {
		ch, err := conn.WaitReady(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("user consume: wait ready failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		if _, err := ch.QueueDeclare(redis_repo.UserPlayCountQueueName, true, false, false, false, nil); err != nil {
			logger.Warn("user consume: queue declare failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		if err := ch.Qos(10, 0, false); err != nil {
			logger.Warn("user consume: qos set failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		msgs, err := ch.Consume(redis_repo.UserPlayCountQueueName, "", false, false, false, false, nil)
		if err != nil {
			logger.Warn("user consume: consume register failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		logger.Info("user consume registered, waiting for messages")

		for msg := range msgs {
			select {
			case <-ctx.Done():
				_ = msg.Nack(false, true)
				return
			default:
			}

			var inc cache.UserPlayCountIncrementMsg
			if err := json.Unmarshal(msg.Body, &inc); err != nil {
				_ = msg.Nack(false, false)
				continue
			}

			mu.Lock()
			(*pending)[inc.UserID] += inc.Increment
			mu.Unlock()

			_ = msg.Ack(false)
		}

		logger.Warn("user consume channel closed, will re-subscribe after reconnect")
	}
}

// likeFlush 将内存中累积的点赞数增量批量写入 MySQL videos.likes_count
// 与播放量 flush 逻辑一致：先复制快照再释放锁，UPDATE 成功后才减去对应增量
func likeFlush(db *gorm.DB, pending *map[uint]int64, mu *sync.Mutex) {
	mu.Lock()
	if len(*pending) == 0 {
		mu.Unlock()
		return
	}
	batch := make(map[uint]int64, len(*pending))
	for k, v := range *pending {
		batch[k] = v
	}
	mu.Unlock()

	var succeededIDs []uint
	for videoID, increment := range batch {
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
		result := db.WithContext(updateCtx).Model(&struct{}{}).Table("videos").
			Where("id = ?", videoID).
			UpdateColumn("likes_count", gorm.Expr("likes_count + ?", increment))
		updateCancel()
		if result.Error != nil {
			zap.L().Error("更新点赞数失败",
				zap.Uint("video_id", videoID),
				zap.Int64("increment", increment),
				zap.Error(result.Error))
		} else {
			succeededIDs = append(succeededIDs, videoID)
		}
	}

	mu.Lock()
	for _, videoID := range succeededIDs {
		increment := batch[videoID]
		if pendingVal, ok := (*pending)[videoID]; ok {
			if pendingVal <= increment {
				delete(*pending, videoID)
			} else {
				(*pending)[videoID] = pendingVal - increment
			}
		}
	}
	mu.Unlock()
}

// likeConsumeLoop 消费 RabbitMQ 中的点赞数增量消息，累加到内存 pending map
// 与 consumeLoop 模式一致：外层重订阅循环 + 内层消息处理循环
func likeConsumeLoop(ctx context.Context, conn *rabbitmq.Connection, logger *zap.Logger, pending *map[uint]int64, mu *sync.Mutex) {
	for {
		ch, err := conn.WaitReady(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("like consume: wait ready failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		if _, err := ch.QueueDeclare(redis_repo.VideoLikeCountQueueName, true, false, false, false, nil); err != nil {
			logger.Warn("like consume: queue declare failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		if err := ch.Qos(10, 0, false); err != nil {
			logger.Warn("like consume: qos set failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		msgs, err := ch.Consume(redis_repo.VideoLikeCountQueueName, "", false, false, false, false, nil)
		if err != nil {
			logger.Warn("like consume: consume register failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		logger.Info("like consume registered, waiting for messages")

		for msg := range msgs {
			select {
			case <-ctx.Done():
				_ = msg.Nack(false, true)
				return
			default:
			}

			var inc cache.VideoLikeIncrementMsg
			if err := json.Unmarshal(msg.Body, &inc); err != nil {
				_ = msg.Nack(false, false)
				continue
			}

			mu.Lock()
			(*pending)[inc.VideoID] += inc.Increment
			mu.Unlock()

			_ = msg.Ack(false)
		}

		logger.Warn("like consume channel closed, will re-subscribe after reconnect")
	}
}
