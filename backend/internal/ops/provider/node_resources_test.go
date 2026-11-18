package provider

import (
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

func TestNodeCPUSamplesUseContainerQuotaAndHostTimeSeparately(t *testing.T) {
	now := time.Now()
	sampler := NewNodeResourceSampler()
	usage := &ops.ResourceUsage{CPUCores: 1.8, HostCPUCores: 2}
	sampler.sampleCPUs(now, 1_000_000_000, true, &cpu.TimesStat{User: 10, Idle: 90}, usage)
	require.Nil(t, usage.CPUUsagePercent)
	require.Nil(t, usage.HostCPUUsagePercent)

	sampler.sampleCPUs(now.Add(time.Second), 2_800_000_000, true, &cpu.TimesStat{User: 12, Idle: 98}, usage)
	require.InDelta(t, 100, *usage.CPUUsagePercent, 0.01)
	require.InDelta(t, 20, *usage.HostCPUUsagePercent, 0.01)

	usage.CPUUsagePercent = nil
	sampler.sampleCPUs(now.Add(2*time.Second), 10, true, &cpu.TimesStat{User: 13, Idle: 99}, usage)
	require.Nil(t, usage.CPUUsagePercent)
}

func TestEffectiveNodeCPUCoresRespectsHostCapacity(t *testing.T) {
	require.Equal(t, 4.0, effectiveNodeCPUCores(4, 0))
	require.Equal(t, 1.8, effectiveNodeCPUCores(2, 1.8))
	require.Equal(t, 2.0, effectiveNodeCPUCores(2, 8))
}
