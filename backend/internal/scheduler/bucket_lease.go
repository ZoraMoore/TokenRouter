package scheduler

import (
	"context"
	"errors"
	"sync"
)

var ErrBucketLeaseLost = errors.New("scheduler bucket lease lost")

// BucketLease 仅归还自己取得的 bucket 锁，共用第一次释放结果。
type BucketLease struct {
	release func(context.Context) error
	once    sync.Once
	err     error
}

func NewBucketLease(release func(context.Context) error) *BucketLease {
	return &BucketLease{release: release}
}

func (l *BucketLease) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.release != nil {
			l.err = l.release(ctx)
		}
	})
	return l.err
}
