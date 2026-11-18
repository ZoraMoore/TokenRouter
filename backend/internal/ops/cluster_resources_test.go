package ops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type resourceCacheStub struct {
	RuntimeCache
	values map[string]string
}

func (c *resourceCacheStub) Get(_ context.Context, key string) (string, error) {
	v, ok := c.values[key]
	if !ok {
		return "", errors.New("missing snapshot")
	}
	return v, nil
}

func (c *resourceCacheStub) Put(_ context.Context, key, value string, _ time.Duration) error {
	c.values[key] = value
	return nil
}

func resourceFixture(id string, now time.Time, cores, cpuPercent float64, memory int64) NodeResources {
	return NodeResources{
		NodeID: id, Name: id, SampledAt: now,
		ResourceUsage: ResourceUsage{
			CPUCores: cores, CPUUsagePercent: &cpuPercent,
			HostCPUCores: cores, HostCPUUsagePercent: &cpuPercent,
			MemoryTotalMB: &memory, MemoryUsedMB: int64Ptr(memory / 2),
			HostMemoryTotalMB: &memory, HostMemoryUsedMB: int64Ptr(memory / 2),
			Disks: &DiskSnapshot{TotalMB: memory * 10, UsedMB: memory * 4, AvailableMB: memory * 6},
		},
	}
}

func TestBuildClusterResourcesWeightsCPUAndSumsCapacity(t *testing.T) {
	now := time.Now().UTC()
	result := buildClusterResources(now, []NodeResources{
		resourceFixture("primary", now, 4, 50, 5120),
		resourceFixture("secondary", now, 2, 10, 3072),
		resourceFixture("tertiary", now, 2, 20, 3072),
	})
	require.Equal(t, 3, result.OnlineNodes)
	require.True(t, result.Complete)
	require.Equal(t, int64(11264), *result.Summary.MemoryTotalMB)
	require.Equal(t, int64(5632), *result.Summary.MemoryUsedMB)
	require.InDelta(t, 32.5, *result.Summary.CPUUsagePercent, 0.01)
	require.Equal(t, int64(112640), result.Summary.Disks.TotalMB)
}

func TestBuildClusterResourcesExcludesOfflineAndFutureSamples(t *testing.T) {
	now := time.Now().UTC()
	result := buildClusterResources(now, []NodeResources{
		resourceFixture("primary", now, 4, 50, 5120),
		resourceFixture("secondary", now.Add(-2*time.Minute), 2, 10, 3072),
		resourceFixture("tertiary", now.Add(time.Minute), 2, 20, 3072),
	})
	require.Equal(t, 1, result.OnlineNodes)
	require.False(t, result.Complete)
	require.Equal(t, int64(5120), *result.Summary.MemoryTotalMB)
	require.False(t, result.Nodes[1].Online)
	require.False(t, result.Nodes[2].Online)
	require.Equal(t, now.Add(-2*time.Minute), result.Nodes[1].SampledAt)
}

func TestBuildClusterResourcesDoesNotTurnMissingMetricsIntoZero(t *testing.T) {
	now := time.Now().UTC()
	node := resourceFixture("primary", now, 4, 50, 5120)
	node.CPUUsagePercent = nil
	node.MemoryUsedMB = nil
	node.Disks = nil
	result := buildClusterResources(now, []NodeResources{node})
	require.Nil(t, result.Summary.CPUUsagePercent)
	require.Nil(t, result.Summary.MemoryUsedMB)
	require.Nil(t, result.Summary.Disks)
	require.Equal(t, int64(5120), *result.Summary.MemoryTotalMB)
}

func TestClusterResourcePublisherAndReaderUseStableNodeIdentity(t *testing.T) {
	cache := &resourceCacheStub{values: map[string]string{}}
	cfg := &Options{ClusterNodeID: "secondary", ClusterNodeName: "135.148.26.72", ClusterNodeIDs: []string{"primary", "secondary", "secondary"}, NodeCache: cache}
	cfg.Ops.Enabled = true
	cfg.ReadNodeResources = func(context.Context) (*ResourceUsage, error) {
		node := resourceFixture("", time.Now(), 2, 10, 3072)
		return &node.ResourceUsage, nil
	}
	collector := &OpsMetricsCollector{cfg: cfg}
	collector.collectNodeResources()
	service := &OpsService{cfg: cfg}
	result := service.readClusterResources(context.Background())
	require.Len(t, result.Nodes, 2)
	require.Equal(t, 1, result.OnlineNodes)
	require.Equal(t, "135.148.26.72", result.Nodes[1].Name)
	require.Equal(t, "secondary", result.Nodes[1].NodeID)

	// 缓存中的身份要与查询的节点一致。
	bad, err := json.Marshal(resourceFixture("primary", time.Now(), 4, 50, 5120))
	require.NoError(t, err)
	cache.values[clusterResourceKey("secondary")] = string(bad)
	require.Equal(t, 0, service.readClusterResources(context.Background()).OnlineNodes)
}
