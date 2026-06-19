package snowflake

import (
	"errors"
	"sync"
	"time"
)

// Snowflake ID 结构（64 位）:
//   - 1 bit: 符号位（始终为 0）
//   - 41 bits: 时间戳（毫秒，自定义纪元起）
//   - 10 bits: 工作机器 ID（0-1023）
//   - 12 bits: 序列号（每毫秒 0-4095）

const (
	workerBits     = 10
	sequenceBits   = 12
	maxWorkerID    = -1 ^ (-1 << workerBits)   // 1023
	maxSequence    = -1 ^ (-1 << sequenceBits) // 4095
	workerShift    = sequenceBits              // 12
	timestampShift = sequenceBits + workerBits // 22

	// 自定义纪元：2024-01-01 00:00:00 UTC（毫秒）
	epoch = 1704067200000
)

var (
	ErrInvalidWorkerID = errors.New("worker ID must be between 0 and 1023")
	ErrClockBackwards  = errors.New("clock moved backwards")
)

// Node Snowflake 节点
type Node struct {
	mu        sync.Mutex
	workerID  int64
	sequence  int64
	lastStamp int64
}

// NewNode 创建 Snowflake 节点
// workerID: 工作机器 ID（0-1023），单机部署用 0 即可
func NewNode(workerID int64) (*Node, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, ErrInvalidWorkerID
	}
	return &Node{workerID: workerID}, nil
}

// Generate 生成唯一 ID
func (n *Node) Generate() (int64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now().UnixMilli() - epoch

	if now < n.lastStamp {
		return 0, ErrClockBackwards
	}

	if now == n.lastStamp {
		n.sequence = (n.sequence + 1) & maxSequence
		if n.sequence == 0 {
			// 当前毫秒序列号用完，等待下一毫秒
			for now <= n.lastStamp {
				now = time.Now().UnixMilli() - epoch
			}
		}
	} else {
		n.sequence = 0
	}

	n.lastStamp = now

	id := (now << timestampShift) | (n.workerID << workerShift) | n.sequence
	return id, nil
}

// 全局默认节点（workerID=0），适合单机部署
var defaultNode, _ = NewNode(0)

// Generate 使用默认节点生成 ID
func Generate() int64 {
	id, _ := defaultNode.Generate()
	return id
}
