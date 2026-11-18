package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

type monitoredDisk struct {
	role        string
	path        string
	displayPath string
	optional    bool
}

// CollectHostDisks 读取系统盘和数据盘，Docker 可通过 host-disks 下的只读挂载提供路径。
func CollectHostDisks(ctx context.Context) (*ops.DiskSnapshot, error) {
	systemPath := hostDiskPath("/host-disks/system", "/")
	dataPath := hostDiskPath("/host-disks/data", "/mnt/data")
	mounted := systemPath == "/host-disks/system" || dataPath == "/host-disks/data"
	_, containerErr := os.Stat("/.dockerenv")
	if (mounted || containerErr == nil) && (systemPath != "/host-disks/system" || dataPath != "/host-disks/data") {
		return nil, fmt.Errorf("host disk mounts are required at /host-disks/system and /host-disks/data")
	}
	paths := []monitoredDisk{
		{role: "system", path: systemPath, displayPath: "/"},
		{role: "data", path: dataPath, displayPath: "/mnt/data", optional: !mounted},
	}
	partitions, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil, err
	}
	return collectDisks(ctx, paths, partitions, disk.UsageWithContext)
}

// hostDiskPath 优先读取部署配置提供的宿主机挂载。
func hostDiskPath(mounted, fallback string) string {
	if info, err := os.Stat(mounted); err == nil && info.IsDir() {
		return mounted
	}
	return fallback
}

// collectDisks 按挂载设备去重，排除绑定挂载造成的重复容量。
func collectDisks(ctx context.Context, paths []monitoredDisk, partitions []disk.PartitionStat, usage func(context.Context, string) (*disk.UsageStat, error)) (*ops.DiskSnapshot, error) {
	snapshot := &ops.DiskSnapshot{Disks: make([]ops.DiskUsage, 0, len(paths))}
	seen := make(map[string]bool)
	for _, candidate := range paths {
		resolved, err := filepath.EvalSymlinks(candidate.path)
		if err != nil {
			if os.IsNotExist(err) && candidate.optional {
				continue
			}
			return nil, fmt.Errorf("read %s disk: %w", candidate.role, err)
		}
		device := diskDevice(resolved, partitions)
		if device == "" {
			return nil, fmt.Errorf("mount device unavailable for %s disk", candidate.role)
		}
		if seen[device] {
			continue
		}
		stat, err := usage(ctx, resolved)
		if err != nil {
			return nil, fmt.Errorf("read %s disk capacity: %w", candidate.role, err)
		}
		if stat == nil || stat.Total == 0 {
			return nil, fmt.Errorf("capacity unavailable for %s disk", candidate.role)
		}
		seen[device] = true
		item := ops.DiskUsage{
			Role: candidate.role, MountPoint: candidate.displayPath,
			TotalMB: int64(stat.Total / bytesPerMB), UsedMB: int64(stat.Used / bytesPerMB),
			AvailableMB: int64(stat.Free / bytesPerMB), UsagePercent: ops.RoundTo1DP(stat.UsedPercent),
		}
		snapshot.Disks = append(snapshot.Disks, item)
		snapshot.TotalMB += item.TotalMB
		snapshot.UsedMB += item.UsedMB
		snapshot.AvailableMB += item.AvailableMB
	}
	return snapshot, nil
}

// diskDevice 按最长挂载路径找到文件系统设备。
func diskDevice(path string, partitions []disk.PartitionStat) string {
	longest := -1
	device := ""
	for _, partition := range partitions {
		mount := filepath.Clean(partition.Mountpoint)
		relative, err := filepath.Rel(mount, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if len(mount) > longest {
			longest = len(mount)
			device = partition.Device
		}
	}
	return device
}
