package provider

import (
	"context"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// NodeResourceSampler 为节点心跳保存独立的 CPU 计数基线。
type NodeResourceSampler struct {
	mu                   sync.Mutex
	lastSampleAt         time.Time
	lastApplicationNanos uint64
	lastApplicationOK    bool
	lastHostTimes        *cpu.TimesStat
}

func NewNodeResourceSampler() *NodeResourceSampler {
	return &NodeResourceSampler{}
}

// Sample 同时读取整机和应用数据，单项采集失败时保留未知值。
func (s *NodeResourceSampler) Sample(ctx context.Context) (*ops.ResourceUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	hostCores := float64(runtime.NumCPU())
	usage := &ops.ResourceUsage{
		HostCPUCores: hostCores,
		CPUCores:     effectiveNodeCPUCores(hostCores, readCgroupCPULimitCores()),
	}
	hostMemory, err := mem.VirtualMemoryWithContext(ctx)
	if err == nil && hostMemory != nil {
		used := int64(hostMemory.Used / bytesPerMB)
		total := int64(hostMemory.Total / bytesPerMB)
		percent := ops.RoundTo1DP(hostMemory.UsedPercent)
		usage.HostMemoryUsedMB = &used
		usage.HostMemoryTotalMB = &total
		usage.HostMemoryUsagePercent = &percent
	}
	used, total, available := readCgroupMemoryBytes()
	usage.MemoryUsedMB, usage.MemoryTotalMB, usage.MemoryUsagePercent = resolveMemoryStats(used, total, available, hostMemory)
	applicationNanos, applicationOK := readCgroupCPUUsageNanos()
	var hostTimes *cpu.TimesStat
	if times, err := cpu.TimesWithContext(ctx, false); err == nil && len(times) == 1 {
		hostTimes = &times[0]
	}
	s.sampleCPUs(now, applicationNanos, applicationOK, hostTimes, usage)
	if disks, err := CollectHostDisks(ctx); err == nil {
		usage.Disks = disks
	}
	return usage, nil
}

func effectiveNodeCPUCores(hostCores, quota float64) float64 {
	if quota > 0 {
		return math.Min(hostCores, quota)
	}
	return hostCores
}

// sampleCPUs 分别计算容器配额使用率和整机使用率，首个样本保留未知值。
func (s *NodeResourceSampler) sampleCPUs(now time.Time, applicationNanos uint64, applicationOK bool, hostTimes *cpu.TimesStat, usage *ops.ResourceUsage) {
	elapsed := now.Sub(s.lastSampleAt).Seconds()
	if !s.lastSampleAt.IsZero() && elapsed > 0 {
		if applicationOK && s.lastApplicationOK && applicationNanos >= s.lastApplicationNanos && usage.CPUCores > 0 {
			value := float64(applicationNanos-s.lastApplicationNanos) / 1e9 / elapsed / usage.CPUCores * 100
			percent := ops.RoundTo1DP(math.Min(100, math.Max(0, value)))
			usage.CPUUsagePercent = &percent
		}
		if hostTimes != nil && s.lastHostTimes != nil {
			total, busy := hostCPUTimes(*hostTimes)
			previousTotal, previousBusy := hostCPUTimes(*s.lastHostTimes)
			if total > previousTotal && busy >= previousBusy {
				percent := ops.RoundTo1DP(math.Min(100, math.Max(0, (busy-previousBusy)/(total-previousTotal)*100)))
				usage.HostCPUUsagePercent = &percent
			}
		}
	}
	s.lastSampleAt = now
	s.lastApplicationNanos = applicationNanos
	s.lastApplicationOK = applicationOK
	if hostTimes != nil {
		copy := *hostTimes
		s.lastHostTimes = &copy
	} else {
		s.lastHostTimes = nil
	}
}

func hostCPUTimes(sample cpu.TimesStat) (total, busy float64) {
	total = sample.User + sample.Nice + sample.System + sample.Idle + sample.Iowait + sample.Irq + sample.Softirq + sample.Steal
	return total, total - sample.Idle - sample.Iowait
}
