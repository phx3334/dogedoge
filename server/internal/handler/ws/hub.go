package ws

import (
	"sync"
)

// DanmakuHub 弹幕房间管理中心，管理所有视频房间的客户端集合
//
// 数据结构：rooms map[videoID]map[*Client]bool
//   - 外层 map：以视频 ID 为 key，每个视频对应一个弹幕房间
//   - 内层 map：以客户端指针为 key（用作 set），存储房间内的所有连接
//   - 使用 sync.RWMutex 保护并发安全
//
// 并发安全策略：
//   - JoinRoom / LeaveRoom：写锁，修改 rooms 结构
//   - BroadcastToRoom：读锁拷贝快照 → 释放锁 → 无锁遍历发送
//     不能直接持读锁遍历，因为 LeaveRoom 的 delete 会并发修改内层 map 导致 panic
type DanmakuHub struct {
	rooms map[uint64]map[*Client]bool
	mu    sync.RWMutex
}

// NewDanmakuHub 创建弹幕房间管理中心
func NewDanmakuHub() *DanmakuHub {
	return &DanmakuHub{
		rooms: make(map[uint64]map[*Client]bool),
	}
}

// JoinRoom 将客户端加入指定视频的弹幕房间
//
// 如果房间不存在则自动创建。同一客户端可以加入不同房间（不同视频）。
func (h *DanmakuHub) JoinRoom(videoID uint64, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[videoID] == nil {
		h.rooms[videoID] = make(map[*Client]bool)
	}
	h.rooms[videoID][client] = true
}

// LeaveRoom 将客户端从指定视频的弹幕房间移除
//
// 如果移除后房间为空（没有其他客户端），自动清理房间，释放内存。
// 由 Client.ReadPump 的 defer 调用，确保连接断开时客户端从房间移除。
func (h *DanmakuHub) LeaveRoom(videoID uint64, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.rooms[videoID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.rooms, videoID)
		}
	}
}

// BroadcastToRoom 向指定视频房间的所有客户端广播消息
//
// 安全性说明：
//   - 先持读锁拷贝 client 指针切片，再释放锁后遍历发送
//   - 不能直接持有读锁遍历 h.rooms[videoID]，因为 LeaveRoom 的 delete 会并发修改 map 导致 panic
//   - client.Close() 使用 sync.Once 保护，BroadcastToRoom 和 ReadPump 可安全并发调用
//
// 背压处理：
//   - 客户端 Send 通道缓冲区满时，异步关闭该客户端连接
//   - 不阻塞广播，避免一个慢客户端影响整个房间的消息推送
func (h *DanmakuHub) BroadcastToRoom(videoID uint64, message []byte) {
	// 持读锁期间拷贝 client 指针，释放锁后再发送，避免持锁期间 map 被并发修改
	h.mu.RLock()
	clients := h.rooms[videoID]
	snapshot := make([]*Client, 0, len(clients))
	for client := range clients {
		snapshot = append(snapshot, client)
	}
	h.mu.RUnlock()

	// 无锁遍历快照发送消息
	for _, client := range snapshot {
		select {
		case client.Send <- message:
		default:
			// 客户端缓冲区满，异步关闭连接
			// 使用 goroutine 避免阻塞广播循环
			go client.Close()
		}
	}
}
