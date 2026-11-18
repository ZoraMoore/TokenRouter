//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

const ethernetProtocolAll = 0x0003

type packetCapture struct {
	fd                 int
	receiveBufferBytes int
	closeOnce          sync.Once
	closeErr           error
}

func openPacketCapture(interfaceName string, receiveBufferBytes int) (*packetCapture, string, error) {
	resolvedName := strings.TrimSpace(interfaceName)
	if resolvedName == "" {
		var err error
		resolvedName, err = defaultRouteInterfaceName()
		if err != nil {
			return nil, "", err
		}
	}
	device, err := net.InterfaceByName(resolvedName)
	if err != nil {
		return nil, "", fmt.Errorf("resolve capture interface %q: %w", resolvedName, err)
	}
	if device.Flags&net.FlagUp == 0 || device.Flags&net.FlagLoopback != 0 {
		return nil, "", fmt.Errorf("capture interface %q must be up and non-loopback", resolvedName)
	}

	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, int(hostToNetworkShort(ethernetProtocolAll)))
	if err != nil {
		return nil, "", fmt.Errorf("open AF_PACKET socket: %w", err)
	}
	capture := &packetCapture{fd: fd}
	if receiveBufferBytes > 0 {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, receiveBufferBytes); err != nil {
			_ = capture.Close()
			return nil, "", fmt.Errorf("set capture receive buffer: %w", err)
		}
	}
	actualReceiveBuffer, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	if err != nil {
		_ = capture.Close()
		return nil, "", fmt.Errorf("read capture receive buffer: %w", err)
	}
	capture.receiveBufferBytes = actualReceiveBuffer
	if err := unix.SetsockoptInt(fd, unix.SOL_PACKET, 23, 1); err != nil { // PACKET_IGNORE_OUTGOING
		_ = capture.Close()
		return nil, "", fmt.Errorf("ignore outgoing packets: %w", err)
	}
	if err := attachIngressTCP443Filter(fd); err != nil {
		_ = capture.Close()
		return nil, "", err
	}
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_LOCK_FILTER, 1); err != nil {
		_ = capture.Close()
		return nil, "", fmt.Errorf("lock tcp/443 packet filter: %w", err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrLinklayer{
		Protocol: hostToNetworkShort(ethernetProtocolAll),
		Ifindex:  device.Index,
	}); err != nil {
		_ = capture.Close()
		return nil, "", fmt.Errorf("bind AF_PACKET socket to %q: %w", resolvedName, err)
	}
	return capture, resolvedName, nil
}

// attachIngressTCP443Filter 使用 tcpdump 对 EN10MB `tcp dst port 443` 生成的经典 BPF。
// 用户态仍会完整校验 IP/TCP/SNI；内核过滤只负责避免复制无关接口流量。
func attachIngressTCP443Filter(fd int) error {
	filters := []unix.SockFilter{
		{Code: 40, Jt: 0, Jf: 0, K: 12},
		{Code: 21, Jt: 0, Jf: 4, K: 34525},
		{Code: 48, Jt: 0, Jf: 0, K: 20},
		{Code: 21, Jt: 0, Jf: 11, K: 6},
		{Code: 40, Jt: 0, Jf: 0, K: 56},
		{Code: 21, Jt: 8, Jf: 9, K: 443},
		{Code: 21, Jt: 0, Jf: 8, K: 2048},
		{Code: 48, Jt: 0, Jf: 0, K: 23},
		{Code: 21, Jt: 0, Jf: 6, K: 6},
		{Code: 40, Jt: 0, Jf: 0, K: 20},
		{Code: 69, Jt: 4, Jf: 0, K: 8191},
		{Code: 177, Jt: 0, Jf: 0, K: 14},
		{Code: 72, Jt: 0, Jf: 0, K: 16},
		{Code: 21, Jt: 0, Jf: 1, K: 443},
		{Code: 6, Jt: 0, Jf: 0, K: 262144},
		{Code: 6, Jt: 0, Jf: 0, K: 0},
	}
	program := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &program); err != nil {
		return fmt.Errorf("attach tcp/443 packet filter: %w", err)
	}
	return nil
}

func (c *packetCapture) ReceiveBufferBytes() int {
	if c == nil {
		return 0
	}
	return c.receiveBufferBytes
}

func (c *packetCapture) Statistics() (uint32, uint32, error) {
	if c == nil || c.fd < 0 {
		return 0, 0, errors.New("packet capture is closed")
	}
	stats, err := unix.GetsockoptTpacketStats(c.fd, unix.SOL_PACKET, unix.PACKET_STATISTICS)
	if err != nil {
		return 0, 0, err
	}
	return stats.Packets, stats.Drops, nil
}

func (c *packetCapture) Read(ctx context.Context, buffer []byte) (int, error) {
	if c == nil || c.fd < 0 {
		return 0, errors.New("packet capture is closed")
	}
	for {
		count, _, err := syscall.Recvfrom(c.fd, buffer, 0)
		if err == nil {
			return count, nil
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			return 0, err
		}
	}
}

func (c *packetCapture) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closeErr = syscall.Close(c.fd)
	})
	return c.closeErr
}

func hostToNetworkShort(value uint16) uint16 {
	var source [2]byte
	binary.NativeEndian.PutUint16(source[:], value)
	return binary.BigEndian.Uint16(source[:])
}

func defaultRouteInterfaceName() (string, error) {
	data, err := os.ReadFile("/proc/net/route")
	if err == nil {
		if interfaceName := selectIPv4DefaultRouteInterface(string(data)); interfaceName != "" {
			return interfaceName, nil
		}
	}
	data, err = os.ReadFile("/proc/net/ipv6_route")
	if err == nil {
		if interfaceName := selectIPv6DefaultRouteInterface(string(data)); interfaceName != "" {
			return interfaceName, nil
		}
	}
	interfaces, listErr := net.Interfaces()
	if listErr != nil {
		return "", fmt.Errorf("discover capture interface: %w", listErr)
	}
	for _, device := range interfaces {
		if device.Flags&net.FlagUp != 0 && device.Flags&net.FlagLoopback == 0 {
			return device.Name, nil
		}
	}
	return "", errors.New("no up non-loopback capture interface found")
}

func selectIPv4DefaultRouteInterface(data string) string {
	bestInterface := ""
	bestMetric := uint64(^uint32(0))
	lines := strings.Split(data, "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" {
			continue
		}
		flags, flagsErr := strconv.ParseUint(fields[3], 16, 32)
		metric, metricErr := strconv.ParseUint(fields[6], 10, 32)
		if flagsErr != nil || metricErr != nil || flags&0x1 == 0 {
			continue
		}
		if bestInterface == "" || metric < bestMetric {
			bestInterface = fields[0]
			bestMetric = metric
		}
	}
	return bestInterface
}

func selectIPv6DefaultRouteInterface(data string) string {
	bestInterface := ""
	bestMetric := uint64(^uint32(0))
	for line := range strings.SplitSeq(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[0] != strings.Repeat("0", 32) || fields[1] != "00" {
			continue
		}
		metric, metricErr := strconv.ParseUint(fields[5], 16, 32)
		flags, flagsErr := strconv.ParseUint(fields[8], 16, 32)
		if metricErr != nil || flagsErr != nil || flags&0x1 == 0 {
			continue
		}
		if bestInterface == "" || metric < bestMetric {
			bestInterface = fields[9]
			bestMetric = metric
		}
	}
	return bestInterface
}
