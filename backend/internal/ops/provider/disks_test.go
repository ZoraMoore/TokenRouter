package provider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/require"
)

func TestCollectDisksDeduplicatesMountedDevices(t *testing.T) {
	system := t.TempDir()
	data := t.TempDir()
	paths := []monitoredDisk{{role: "system", path: system, displayPath: "/"}, {role: "data", path: data, displayPath: "/mnt/data"}, {role: "data", path: system, displayPath: "/bind-copy"}}
	partitions := []disk.PartitionStat{{Device: "/dev/sda1", Mountpoint: system}, {Device: "/dev/sdb", Mountpoint: data}}
	calls := 0
	snapshot, err := collectDisks(context.Background(), paths, partitions, func(_ context.Context, path string) (*disk.UsageStat, error) {
		calls++
		total, used := uint64(72*testGiB), uint64(21*testGiB)
		if path == data {
			total, used = 98*testGiB, 19*testGiB
		}
		return &disk.UsageStat{Total: total, Used: used, Free: total - used, UsedPercent: 100 * float64(used) / float64(total)}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, snapshot.Disks, 2)
	require.Equal(t, int64(170*1024), snapshot.TotalMB)
	require.Equal(t, int64(40*1024), snapshot.UsedMB)
	require.Equal(t, int64(130*1024), snapshot.AvailableMB)
}

func TestCollectDisksReportsUnreadableCapacity(t *testing.T) {
	root := t.TempDir()
	_, err := collectDisks(context.Background(), []monitoredDisk{{role: "system", path: root}}, []disk.PartitionStat{{Device: "/dev/sda1", Mountpoint: root}}, func(context.Context, string) (*disk.UsageStat, error) {
		return nil, errors.New("permission denied")
	})
	require.ErrorContains(t, err, "system disk capacity")
}

func TestDiskDeviceUsesLongestMountAndPathBoundary(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	mounts := []disk.PartitionStat{{Device: "system", Mountpoint: root}, {Device: "data", Mountpoint: data}}
	require.Equal(t, "data", diskDevice(filepath.Join(data, "nested"), mounts))
	require.Equal(t, "system", diskDevice(filepath.Join(root, "data-other"), mounts))
}
