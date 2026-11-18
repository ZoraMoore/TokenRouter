//go:build linux

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectIPv4DefaultRouteInterfaceUsesLowestUpMetric(t *testing.T) {
	t.Parallel()
	routes := `Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
eth1 00000000 0100000A 0003 0 0 200 00000000 0 0 0
eth0 00000000 0100000A 0003 0 0 100 00000000 0 0 0
down0 00000000 00000000 0000 0 0 1 00000000 0 0 0
`
	require.Equal(t, "eth0", selectIPv4DefaultRouteInterface(routes))
}

func TestSelectIPv6DefaultRouteInterfaceUsesLowestUpMetric(t *testing.T) {
	t.Parallel()
	routes := `00000000000000000000000000000000 00 00000000000000000000000000000000 00 20010db8000000000000000000000001 00000400 00000000 00000000 00000001 eth1
00000000000000000000000000000000 00 00000000000000000000000000000000 00 20010db8000000000000000000000001 00000064 00000000 00000000 00000001 eth0
`
	require.Equal(t, "eth0", selectIPv6DefaultRouteInterface(routes))
}
