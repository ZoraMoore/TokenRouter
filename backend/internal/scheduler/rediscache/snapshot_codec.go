package rediscache

import (
	"time"

	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

// SnapshotCodec 编解码快照记录，SnapshotCache 管理 Redis 键、版本、锁和发布顺序。
type SnapshotCodec interface {
	Encode(scheduler.SnapshotProvider) ([]byte, []byte, error)
	Decode(any) (scheduler.SnapshotProvider, error)
	LastUsedAt(scheduler.SnapshotProvider) (*time.Time, error)
	SetLastUsedAt(scheduler.SnapshotProvider, *time.Time) error
}
