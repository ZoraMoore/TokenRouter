package ops

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
)

const (
	clusterResourceInterval  = 15 * time.Second
	clusterResourceFreshness = time.Minute
	clusterResourceRetention = 24 * time.Hour
	clusterResourceMaxNodes  = 32
)

// ResourceUsage 区分宿主机资源与应用容器额度，容量字段使用 MiB。
type ResourceUsage struct {
	CPUCores               float64       `json:"cpu_cores"`
	CPUUsagePercent        *float64      `json:"cpu_usage_percent"`
	MemoryUsedMB           *int64        `json:"memory_used_mb"`
	MemoryTotalMB          *int64        `json:"memory_total_mb"`
	MemoryUsagePercent     *float64      `json:"memory_usage_percent"`
	HostCPUCores           float64       `json:"host_cpu_cores"`
	HostCPUUsagePercent    *float64      `json:"host_cpu_usage_percent"`
	HostMemoryUsedMB       *int64        `json:"host_memory_used_mb"`
	HostMemoryTotalMB      *int64        `json:"host_memory_total_mb"`
	HostMemoryUsagePercent *float64      `json:"host_memory_usage_percent"`
	Disks                  *DiskSnapshot `json:"disks"`
}

// NodeResources 使用部署指定的稳定节点标识保存最后一次采样。
type NodeResources struct {
	ResourceUsage
	NodeID    string    `json:"node_id"`
	Name      string    `json:"name"`
	SampledAt time.Time `json:"sampled_at"`
	Online    bool      `json:"online"`
}

// ClusterResources 汇总在线节点，离线节点保留在明细中供定位。
type ClusterResources struct {
	TotalNodes  int             `json:"total_nodes"`
	OnlineNodes int             `json:"online_nodes"`
	Complete    bool            `json:"complete"`
	Summary     ResourceUsage   `json:"summary"`
	Nodes       []NodeResources `json:"nodes"`
}

func clusterResourceKey(id string) string {
	return "ops:node:resources:v1:" + id
}

// runNodeResources 在每个实例采样，使用独立键保存各节点的快照。
func (c *OpsMetricsCollector) runNodeResources() {
	if c.cfg == nil || c.cfg.ClusterNodeID == "" || c.cfg.NodeCache == nil || c.cfg.ReadNodeResources == nil {
		return
	}
	c.collectNodeResources()
	ticker := time.NewTicker(clusterResourceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.collectNodeResources()
		case <-c.stopCh:
			return
		}
	}
}

func (c *OpsMetricsCollector) collectNodeResources() {
	if c == nil || c.cfg == nil || c.cfg.NodeCache == nil || c.cfg.ReadNodeResources == nil || c.cfg.ClusterNodeID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if !c.isMonitoringEnabled(ctx) {
		return
	}
	usage, err := c.cfg.ReadNodeResources(ctx)
	if err != nil || usage == nil {
		return
	}
	node := NodeResources{ResourceUsage: *usage, NodeID: c.cfg.ClusterNodeID, Name: c.cfg.ClusterNodeName, SampledAt: time.Now().UTC()}
	if node.Name == "" {
		node.Name = node.NodeID
	}
	raw, err := json.Marshal(node)
	if err == nil {
		_ = c.cfg.NodeCache.Put(ctx, clusterResourceKey(node.NodeID), string(raw), clusterResourceRetention)
	}
}

func (s *OpsService) readClusterResources(ctx context.Context) *ClusterResources {
	if s.cfg == nil || s.cfg.NodeCache == nil || len(s.cfg.ClusterNodeIDs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	nodes := make([]NodeResources, 0, len(s.cfg.ClusterNodeIDs))
	seen := make(map[string]bool)
	for _, id := range s.cfg.ClusterNodeIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] || len(id) > 80 || len(nodes) >= clusterResourceMaxNodes {
			continue
		}
		seen[id] = true
		node := NodeResources{NodeID: id, Name: id}
		if raw, err := s.cfg.NodeCache.Get(ctx, clusterResourceKey(id)); err == nil && len(raw) <= 64*1024 {
			var stored NodeResources
			if json.Unmarshal([]byte(raw), &stored) == nil && stored.NodeID == id {
				node = stored
			}
		}
		nodes = append(nodes, node)
	}
	return buildClusterResources(time.Now().UTC(), nodes)
}

func buildClusterResources(now time.Time, nodes []NodeResources) *ClusterResources {
	result := &ClusterResources{TotalNodes: len(nodes), Nodes: append([]NodeResources(nil), nodes...)}
	var online []ResourceUsage
	for i := range result.Nodes {
		node := &result.Nodes[i]
		age := now.Sub(node.SampledAt)
		node.Online = !node.SampledAt.IsZero() && age >= -5*time.Second && age <= clusterResourceFreshness
		if node.Online {
			result.OnlineNodes++
			online = append(online, node.ResourceUsage)
		}
	}
	result.Complete = result.TotalNodes > 0 && result.TotalNodes == result.OnlineNodes
	if len(online) == 0 {
		return result
	}
	for _, usage := range online {
		result.Summary.CPUCores += usage.CPUCores
		result.Summary.HostCPUCores += usage.HostCPUCores
	}
	result.Summary.CPUUsagePercent = aggregateResourceCPU(online, false)
	result.Summary.HostCPUUsagePercent = aggregateResourceCPU(online, true)
	result.Summary.MemoryUsedMB = aggregateResourceMemory(online, func(u ResourceUsage) *int64 { return u.MemoryUsedMB })
	result.Summary.MemoryTotalMB = aggregateResourceMemory(online, func(u ResourceUsage) *int64 { return u.MemoryTotalMB })
	result.Summary.HostMemoryUsedMB = aggregateResourceMemory(online, func(u ResourceUsage) *int64 { return u.HostMemoryUsedMB })
	result.Summary.HostMemoryTotalMB = aggregateResourceMemory(online, func(u ResourceUsage) *int64 { return u.HostMemoryTotalMB })
	result.Summary.MemoryUsagePercent = resourceMemoryPercent(result.Summary.MemoryUsedMB, result.Summary.MemoryTotalMB)
	result.Summary.HostMemoryUsagePercent = resourceMemoryPercent(result.Summary.HostMemoryUsedMB, result.Summary.HostMemoryTotalMB)
	result.Summary.Disks = aggregateResourceDisks(online)
	return result
}

// aggregateResourceCPU 按核心数加权，避免把不同容量节点的百分比直接相加。
func aggregateResourceCPU(nodes []ResourceUsage, host bool) *float64 {
	var capacity, used float64
	for _, node := range nodes {
		cores, percent := node.CPUCores, node.CPUUsagePercent
		if host {
			cores, percent = node.HostCPUCores, node.HostCPUUsagePercent
		}
		if cores <= 0 || percent == nil || math.IsNaN(*percent) || math.IsInf(*percent, 0) {
			return nil
		}
		capacity += cores
		used += cores * math.Min(100, math.Max(0, *percent)) / 100
	}
	return float64Ptr(RoundTo1DP(used / capacity * 100))
}

func aggregateResourceMemory(nodes []ResourceUsage, value func(ResourceUsage) *int64) *int64 {
	var total int64
	for _, node := range nodes {
		v := value(node)
		if v == nil || *v < 0 {
			return nil
		}
		total += *v
	}
	return &total
}

func resourceMemoryPercent(used, total *int64) *float64 {
	if used == nil || total == nil || *total <= 0 {
		return nil
	}
	return float64Ptr(RoundTo1DP(float64(*used) / float64(*total) * 100))
}

// aggregateResourceDisks 使用各主机已去重的总量，相同设备名在不同主机分别计数。
func aggregateResourceDisks(nodes []ResourceUsage) *DiskSnapshot {
	result := &DiskSnapshot{Disks: []DiskUsage{}}
	for _, node := range nodes {
		if node.Disks == nil {
			return nil
		}
		result.TotalMB += node.Disks.TotalMB
		result.UsedMB += node.Disks.UsedMB
		result.AvailableMB += node.Disks.AvailableMB
	}
	return result
}
