package operation

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// TaskStore 保存任务请求，供其他进程读取和执行。
type TaskStore interface {
	Enqueue(context.Context, Job) (Job, error)
	Active(context.Context, int64) (Job, error)
	Get(context.Context, string) (Job, error)
	List(context.Context, int64) ([]Job, error)
}

// Service 接收管理员选择的账号，为每个合格账号排入一种操作。
type Service struct {
	tasks   TaskStore
	relogin func(context.Context, int64) (PreviewItem, error)
	traffic func(context.Context, int64, []int64) error
	name    func(context.Context, int64) (string, error)
}

// NewService 绑定任务读写和两类账号校验。
func NewService(tasks TaskStore, relogin func(context.Context, int64) (PreviewItem, error), traffic func(context.Context, int64, []int64) error, name func(context.Context, int64) (string, error)) *Service {
	return &Service{tasks: tasks, relogin: relogin, traffic: traffic, name: name}
}

// PreviewRelogin 按勾选顺序返回结果，限制访问外部档案的并发。
func (s *Service) PreviewRelogin(ctx context.Context, ids []int64) []PreviewItem {
	items := make([]PreviewItem, len(ids))
	indices := make(chan int)
	var group sync.WaitGroup
	for range min(5, len(ids)) {
		group.Go(func() {
			for i := range indices {
				items[i] = s.previewRelogin(ctx, ids[i])
			}
		})
	}
	for i := range ids {
		indices <- i
	}
	close(indices)
	group.Wait()
	return items
}

func (s *Service) previewRelogin(ctx context.Context, id int64) PreviewItem {
	current, err := s.tasks.Active(ctx, id)
	if err == nil {
		item := PreviewItem{ProviderID: id, Name: current.ProviderName, Eligible: current.Kind == Relogin}
		if !item.Eligible {
			item.Reason = "该账号有在途推流任务"
		}
		return item
	}
	item := PreviewItem{ProviderID: id}
	if errors.Is(err, ErrNotFound) {
		item, err = s.relogin(ctx, id)
		item.ProviderID = id
	}
	if err != nil {
		item.Eligible = false
		item.Reason = publicReason(err, "账号核验暂时失败")
	}
	return item
}

// SubmitRelogin 提交当前仍合格的账号，按账号返回跳过原因。
func (s *Service) SubmitRelogin(ctx context.Context, ids []int64) ([]Job, []PreviewItem) {
	items := s.PreviewRelogin(ctx, ids)
	jobs := make([]Job, 0)
	skipped := make([]PreviewItem, 0)
	for _, item := range items {
		if item.Eligible {
			job, err := s.enqueue(ctx, Job{ProviderID: item.ProviderID, ProviderName: item.Name, Kind: Relogin})
			if err == nil {
				jobs = append(jobs, job)
				continue
			}
			item.Eligible = false
			item.Reason = publicReason(err, "无法保存重登任务")
		}
		skipped = append(skipped, item)
	}
	return jobs, skipped
}

// SubmitTraffic 按保存后的账号和管理员选择的分组校验推流资格。
func (s *Service) SubmitTraffic(ctx context.Context, providerID int64, groupIDs []int64) (Job, error) {
	if len(groupIDs) == 0 || len(groupIDs) > 100 {
		return Job{}, NeedsAction("请选择 1 到 100 个推流来源分组")
	}
	groupIDs = slices.Clone(groupIDs)
	slices.Sort(groupIDs)
	groupIDs = slices.Compact(groupIDs)
	if groupIDs[0] <= 0 {
		return Job{}, NeedsAction("推流来源分组 ID 必须为正整数")
	}
	current, err := s.tasks.Active(ctx, providerID)
	if err == nil {
		if current.Kind == TrafficPush && slices.Equal(current.TrafficGroups(), groupIDs) {
			return current, nil
		}
		return Job{}, ErrBusy
	}
	if !errors.Is(err, ErrNotFound) {
		return Job{}, err
	}
	if err := s.traffic(ctx, providerID, groupIDs); err != nil {
		return Job{}, err
	}
	name, err := s.name(ctx, providerID)
	if err != nil {
		return Job{}, err
	}
	job := Job{ProviderID: providerID, ProviderName: name, Kind: TrafficPush, GroupIDs: groupIDs}
	if len(groupIDs) == 1 {
		job.GroupID = groupIDs[0]
		job.GroupIDs = nil
	}
	return s.enqueue(ctx, job)
}

// Get 查询单项持久化进度。
func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	return s.tasks.Get(ctx, id)
}

// List 读取最近任务，providerID 为 0 时列出全部账号。
func (s *Service) List(ctx context.Context, providerID int64) ([]Job, error) {
	return s.tasks.List(ctx, providerID)
}

func (s *Service) enqueue(ctx context.Context, job Job) (Job, error) {
	saved, err := s.tasks.Enqueue(ctx, job)
	if err != nil {
		return Job{}, err
	}
	if saved.Kind != job.Kind || !slices.Equal(saved.TrafficGroups(), job.TrafficGroups()) {
		return Job{}, ErrBusy
	}
	return saved, nil
}

func publicReason(err error, fallback string) string {
	if errors.Is(err, ErrBusy) {
		return "该账号已有其他在途任务"
	}
	if message, ok := PublicMessage(err); ok {
		return message
	}
	return fallback
}
