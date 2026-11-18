package main

import (
	"encoding/binary"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecodeEthernetTCPPacketIPv4(t *testing.T) {
	t.Parallel()
	payload := []byte{22, 3, 1, 0, 0}
	frame := observerIPv4TCPFrame(t, "192.0.2.10", "203.0.113.20", 52000, 443, 100, 0x18, payload)
	segment, ok := decodeEthernetTCPPacket(frame, 443)
	require.True(t, ok)
	require.Equal(t, netip.MustParseAddr("192.0.2.10"), segment.flow.sourceAddress)
	require.Equal(t, netip.MustParseAddr("203.0.113.20"), segment.flow.destinationAddress)
	require.Equal(t, uint16(52000), segment.flow.sourcePort)
	require.Equal(t, uint32(100), segment.sequence)
	require.Equal(t, payload, segment.payload)
}

func TestDecodeEthernetTCPPacketIPv6WithDestinationOptions(t *testing.T) {
	t.Parallel()
	payload := []byte{22, 3, 1, 0, 0}
	frame := observerIPv6TCPFrameWithDestinationOptions(t, "2001:db8::10", "2001:db8::20", 52000, 443, 100, payload)
	segment, ok := decodeEthernetTCPPacket(frame, 443)
	require.True(t, ok)
	require.Equal(t, netip.MustParseAddr("2001:db8::10"), segment.flow.sourceAddress)
	require.Equal(t, netip.MustParseAddr("2001:db8::20"), segment.flow.destinationAddress)
	require.Equal(t, payload, segment.payload)
}

func TestFlowAssemblerReassemblesOutOfOrderAndRetransmittedClientHello(t *testing.T) {
	t.Parallel()
	secret := strings.Repeat("s", 32)
	assembler := newObserverTestAssembler(t, secret, defaultMaxFlowBytes)
	flow := observerTestFlow()
	now := time.Unix(1700000000, 0)
	record := observerTestClientHello("api.scienceedu.me")
	firstEnd := len(record) / 3
	secondEnd := firstEnd * 2
	first := record[:firstEnd]
	second := record[firstEnd:secondEnd]
	third := record[secondEnd:]

	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 1000, syn: true, payload: first}, now))
	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 1001 + uint32(secondEnd), payload: third}, now.Add(time.Millisecond)))
	// 完全相同的 TCP 重传不能增加缓冲区，也不能被计算为新样本。
	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 1001 + uint32(secondEnd), payload: third}, now.Add(2*time.Millisecond)))
	observation := assembler.add(tcpSegment{flow: flow, sequence: 1001 + uint32(len(first)), payload: second}, now.Add(3*time.Millisecond))
	require.NotNil(t, observation)
	require.Equal(t, "api.scienceedu.me", observation.ServerName)
	require.Equal(t, 1, observation.SchemaVersion)
	require.NotEmpty(t, observation.FlowID)
	require.NotNil(t, observation.ClientHello)
	require.NotEmpty(t, observation.ClientHello.JA3Hash)
	require.Zero(t, assembler.bufferedBytes)
	// 同一 TCP flow 的后续数据不能重复提交观测。
	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 1001 + uint32(len(record)), payload: []byte{1, 2, 3}}, now.Add(5*time.Millisecond)))
}

func TestFlowAssemblerRejectsWrongSNI(t *testing.T) {
	t.Parallel()
	assembler := newObserverTestAssembler(t, strings.Repeat("s", 32), defaultMaxFlowBytes)
	flow := observerTestFlow()
	record := observerTestClientHello("other.example")
	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 50, payload: record}, time.Now()))
	require.Empty(t, assembler.flows)
}

func TestFlowAssemblerEnforcesPerFlowByteLimit(t *testing.T) {
	t.Parallel()
	record := observerTestClientHello("api.scienceedu.me")
	assembler := newObserverTestAssembler(t, strings.Repeat("s", 32), len(record)-1)
	flow := observerTestFlow()
	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 50, payload: record}, time.Now()))
	require.Empty(t, assembler.flows)
	require.Zero(t, assembler.bufferedBytes)
}

func TestFlowAssemblerExpiresIncompleteFlow(t *testing.T) {
	t.Parallel()
	assembler := newObserverTestAssembler(t, strings.Repeat("s", 32), defaultMaxFlowBytes)
	flow := observerTestFlow()
	started := time.Unix(1700000000, 0)
	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 100, payload: []byte{22, 3, 1, 0, 10}}, started))
	require.Len(t, assembler.flows, 1)
	assembler.cleanup(started.Add(defaultFlowTTL + time.Second))
	require.Empty(t, assembler.flows)
	require.Zero(t, assembler.bufferedBytes)
}

func TestFlowAssemblerDoesNotAllocateForBareSYN(t *testing.T) {
	t.Parallel()
	assembler := newObserverTestAssembler(t, strings.Repeat("s", 32), defaultMaxFlowBytes)
	require.Nil(t, assembler.add(tcpSegment{flow: observerTestFlow(), sequence: 100, syn: true}, time.Now()))
	require.Empty(t, assembler.flows)
	require.Zero(t, assembler.bufferedBytes)
}

func TestFlowAssemblerApplicationDataDoesNotRefreshEmittedTombstone(t *testing.T) {
	t.Parallel()
	assembler := newObserverTestAssembler(t, strings.Repeat("s", 32), defaultMaxFlowBytes)
	flow := observerTestFlow()
	started := time.Unix(1700000000, 0)
	observation := assembler.add(tcpSegment{flow: flow, sequence: 50, payload: observerTestClientHello("api.scienceedu.me")}, started)
	require.NotNil(t, observation)
	originalUpdate := assembler.flows[flow].updatedAt

	require.Nil(t, assembler.add(tcpSegment{flow: flow, sequence: 500, payload: []byte("encrypted application data")}, started.Add(time.Minute)))
	require.Equal(t, originalUpdate, assembler.flows[flow].updatedAt)
	assembler.cleanup(started.Add(defaultFlowTTL + time.Second))
	require.NotContains(t, assembler.flows, flow)
}

func newObserverTestAssembler(t *testing.T, secret string, maxFlowBytes int) *flowAssembler {
	t.Helper()
	assembler, err := newFlowAssembler(assemblerLimits{
		ttl:              defaultFlowTTL,
		maxFlows:         8,
		maxFlowBytes:     maxFlowBytes,
		maxBufferedBytes: maxFlowBytes * 2,
		serverName:       "api.scienceedu.me",
		observerToken:    secret,
	})
	require.NoError(t, err)
	return assembler
}

func observerTestFlow() flowTuple {
	return flowTuple{
		sourceAddress:      netip.MustParseAddr("192.0.2.10"),
		sourcePort:         52000,
		destinationAddress: netip.MustParseAddr("203.0.113.20"),
		destinationPort:    443,
	}
}

func observerTestClientHello(serverName string) []byte {
	name := []byte(serverName)
	sni := make([]byte, 5+len(name))
	binary.BigEndian.PutUint16(sni[0:2], uint16(3+len(name)))
	binary.BigEndian.PutUint16(sni[3:5], uint16(len(name)))
	copy(sni[5:], name)
	extensions := make([]byte, 4+len(sni))
	binary.BigEndian.PutUint16(extensions[2:4], uint16(len(sni)))
	copy(extensions[4:], sni)
	body := make([]byte, 0, 64+len(extensions))
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0)
	body = append(body, 0, 2, 0x13, 0x01)
	body = append(body, 1, 0)
	body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
	body = append(body, extensions...)
	handshake := []byte{1, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	handshake = append(handshake, body...)
	record := []byte{22, 3, 1, byte(len(handshake) >> 8), byte(len(handshake))}
	return append(record, handshake...)
}

func observerIPv4TCPFrame(t *testing.T, source, destination string, sourcePort, destinationPort uint16, sequence uint32, flags byte, payload []byte) []byte {
	t.Helper()
	sourceAddress := netip.MustParseAddr(source).As4()
	destinationAddress := netip.MustParseAddr(destination).As4()
	frame := make([]byte, ethernetHeaderLength+20+20+len(payload))
	binary.BigEndian.PutUint16(frame[12:14], etherTypeIPv4)
	ip := frame[ethernetHeaderLength:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(20+20+len(payload)))
	ip[8] = 64
	ip[9] = ipProtocolTCP
	copy(ip[12:16], sourceAddress[:])
	copy(ip[16:20], destinationAddress[:])
	tcp := ip[20:]
	binary.BigEndian.PutUint16(tcp[0:2], sourcePort)
	binary.BigEndian.PutUint16(tcp[2:4], destinationPort)
	binary.BigEndian.PutUint32(tcp[4:8], sequence)
	tcp[12] = 5 << 4
	tcp[13] = flags
	copy(tcp[20:], payload)
	return frame
}

func observerIPv6TCPFrameWithDestinationOptions(t *testing.T, source, destination string, sourcePort, destinationPort uint16, sequence uint32, payload []byte) []byte {
	t.Helper()
	sourceAddress := netip.MustParseAddr(source).As16()
	destinationAddress := netip.MustParseAddr(destination).As16()
	const extensionLength = 8
	frame := make([]byte, ethernetHeaderLength+40+extensionLength+20+len(payload))
	binary.BigEndian.PutUint16(frame[12:14], etherTypeIPv6)
	ip := frame[ethernetHeaderLength:]
	ip[0] = 0x60
	binary.BigEndian.PutUint16(ip[4:6], uint16(extensionLength+20+len(payload)))
	ip[6] = 60
	ip[7] = 64
	copy(ip[8:24], sourceAddress[:])
	copy(ip[24:40], destinationAddress[:])
	ip[40] = ipProtocolTCP
	ip[41] = 0
	tcp := ip[40+extensionLength:]
	binary.BigEndian.PutUint16(tcp[0:2], sourcePort)
	binary.BigEndian.PutUint16(tcp[2:4], destinationPort)
	binary.BigEndian.PutUint32(tcp[4:8], sequence)
	tcp[12] = 5 << 4
	tcp[13] = 0x18
	copy(tcp[20:], payload)
	return frame
}
