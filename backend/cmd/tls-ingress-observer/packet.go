package main

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
)

const (
	ethernetHeaderLength = 14
	etherTypeIPv4        = 0x0800
	etherTypeIPv6        = 0x86dd
	etherTypeVLAN        = 0x8100
	etherTypeQinQ        = 0x88a8
	etherTypeVLAN9100    = 0x9100
	ipProtocolTCP        = 6

	maxPendingSegmentsPerFlow = 32

	tlsRecordTypeHandshake = 22
)

type flowTuple struct {
	sourceAddress      netip.Addr
	sourcePort         uint16
	destinationAddress netip.Addr
	destinationPort    uint16
}

type tcpSegment struct {
	flow     flowTuple
	sequence uint32
	syn      bool
	fin      bool
	rst      bool
	payload  []byte
}

type pendingSegment struct {
	sequence uint32
	payload  []byte
}

type flowState struct {
	nextSequence uint32
	initialized  bool
	emitted      bool
	updatedAt    time.Time
	buffer       []byte
	pending      []pendingSegment
}

type assemblerLimits struct {
	ttl              time.Duration
	maxFlows         int
	maxFlowBytes     int
	maxBufferedBytes int
	serverName       string
	observerToken    string
}

type flowAssembler struct {
	limits        assemblerLimits
	flows         map[flowTuple]*flowState
	bufferedBytes int
	lastCleanup   time.Time
}

func newFlowAssembler(limits assemblerLimits) (*flowAssembler, error) {
	limits.serverName = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(limits.serverName), "."))
	if limits.ttl <= 0 {
		return nil, errors.New("flow TTL must be positive")
	}
	if limits.maxFlows <= 0 || limits.maxFlowBytes <= 0 || limits.maxBufferedBytes < limits.maxFlowBytes {
		return nil, errors.New("invalid flow assembler limits")
	}
	if limits.serverName == "" {
		return nil, errors.New("capture server name is required")
	}
	if len(limits.observerToken) < 32 {
		return nil, errors.New("observer token must be at least 32 bytes")
	}
	return &flowAssembler{limits: limits, flows: make(map[flowTuple]*flowState)}, nil
}

func (a *flowAssembler) add(segment tcpSegment, observedAt time.Time) *tlsfingerprint.AuthenticatedIngressObservation {
	if !segment.flow.sourceAddress.IsValid() || !segment.flow.destinationAddress.IsValid() {
		return nil
	}
	if segment.flow.destinationPort != 443 {
		return nil
	}
	if a.lastCleanup.IsZero() || observedAt.Sub(a.lastCleanup) >= time.Second {
		a.cleanup(observedAt)
	}
	// 仅 SYN 不携带任何 ClientHello 证据，不能占用长 TTL flow 槽位。
	// 首个 TLS payload 到达时仍可按 sequence 建立有界重组状态。
	if segment.syn && len(segment.payload) == 0 {
		return nil
	}

	state := a.flows[segment.flow]
	if segment.syn {
		if state != nil {
			a.releaseState(state)
			delete(a.flows, segment.flow)
		}
		if len(a.flows) >= a.limits.maxFlows {
			return nil
		}
		state = &flowState{
			nextSequence: segment.sequence + 1,
			initialized:  true,
			updatedAt:    observedAt,
		}
		a.flows[segment.flow] = state
	}
	if state == nil {
		// observer 热启动时可能错过 SYN；只从明显的 TLS handshake record 起点接管。
		if len(segment.payload) < 5 || segment.payload[0] != tlsRecordTypeHandshake {
			return nil
		}
		if len(a.flows) >= a.limits.maxFlows {
			return nil
		}
		state = &flowState{
			nextSequence: segment.sequence,
			initialized:  true,
			updatedAt:    observedAt,
		}
		a.flows[segment.flow] = state
	}
	if state.emitted {
		if segment.fin || segment.rst {
			delete(a.flows, segment.flow)
		}
		return nil
	}
	state.updatedAt = observedAt

	payloadSequence := segment.sequence
	if segment.syn {
		payloadSequence++
	}
	if len(segment.payload) > 0 && !a.acceptPayload(state, payloadSequence, segment.payload) {
		a.dropFlow(segment.flow)
		return nil
	}
	if segment.fin || segment.rst {
		a.dropFlow(segment.flow)
		return nil
	}
	if len(state.buffer) < 5 {
		return nil
	}
	if state.buffer[0] != tlsRecordTypeHandshake {
		a.dropFlow(segment.flow)
		return nil
	}

	captured, err := tlsfingerprint.ParseCapturedClientHelloRecords(state.buffer)
	if err != nil {
		if errors.Is(err, tlsfingerprint.ErrIncompleteClientHello) {
			return nil
		}
		a.dropFlow(segment.flow)
		return nil
	}
	serverName, err := tlsfingerprint.ExtractCapturedClientHelloServerName(state.buffer)
	if err != nil || !strings.EqualFold(serverName, a.limits.serverName) {
		a.dropFlow(segment.flow)
		return nil
	}
	flowID, err := tlsfingerprint.BuildAuthenticatedIngressFlowID(
		a.limits.observerToken,
		segment.flow.sourceAddress.String(),
		segment.flow.sourcePort,
		segment.flow.destinationAddress.String(),
		segment.flow.destinationPort,
	)
	if err != nil {
		a.dropFlow(segment.flow)
		return nil
	}

	a.releaseStateBuffers(state)
	state.emitted = true
	return &tlsfingerprint.AuthenticatedIngressObservation{
		SchemaVersion: tlsfingerprint.AuthenticatedIngressObservationSchemaVersion,
		FlowID:        flowID,
		ObservedAt:    observedAt.UTC(),
		ServerName:    serverName,
		ClientHello:   captured,
	}
}

func (a *flowAssembler) acceptPayload(state *flowState, sequence uint32, payload []byte) bool {
	if !state.initialized || len(payload) == 0 {
		return true
	}
	if sequenceAfter(sequence, state.nextSequence) {
		return a.storePending(state, sequence, payload)
	}
	if sequenceBefore(sequence, state.nextSequence) {
		overlap := int(state.nextSequence - sequence)
		if overlap >= len(payload) {
			return true
		}
		payload = payload[overlap:]
		sequence = state.nextSequence
	}
	if sequence != state.nextSequence || !a.appendContiguous(state, payload) {
		return false
	}
	for {
		index := a.nextPendingIndex(state)
		if index < 0 {
			break
		}
		pending := state.pending[index]
		state.pending = append(state.pending[:index], state.pending[index+1:]...)
		pendingBytes := len(pending.payload)
		a.bufferedBytes -= pendingBytes
		if sequenceBefore(pending.sequence, state.nextSequence) {
			overlap := int(state.nextSequence - pending.sequence)
			if overlap >= len(pending.payload) {
				continue
			}
			pending.payload = pending.payload[overlap:]
		}
		if !a.appendContiguous(state, pending.payload) {
			return false
		}
	}
	return true
}

func (a *flowAssembler) appendContiguous(state *flowState, payload []byte) bool {
	if len(payload) == 0 {
		return true
	}
	if len(state.buffer)+len(payload) > a.limits.maxFlowBytes || a.bufferedBytes+len(payload) > a.limits.maxBufferedBytes {
		return false
	}
	state.buffer = append(state.buffer, payload...)
	state.nextSequence += uint32(len(payload))
	stateBytes := len(payload)
	a.bufferedBytes += stateBytes
	return true
}

func (a *flowAssembler) storePending(state *flowState, sequence uint32, payload []byte) bool {
	for _, pending := range state.pending {
		if pending.sequence == sequence {
			return true
		}
	}
	if len(state.pending) >= maxPendingSegmentsPerFlow || len(state.buffer)+a.pendingBytes(state)+len(payload) > a.limits.maxFlowBytes || a.bufferedBytes+len(payload) > a.limits.maxBufferedBytes {
		return false
	}
	copyOfPayload := append([]byte(nil), payload...)
	state.pending = append(state.pending, pendingSegment{sequence: sequence, payload: copyOfPayload})
	a.bufferedBytes += len(copyOfPayload)
	return true
}

func (a *flowAssembler) nextPendingIndex(state *flowState) int {
	for index, pending := range state.pending {
		if pending.sequence == state.nextSequence || sequenceBefore(pending.sequence, state.nextSequence) {
			return index
		}
	}
	return -1
}

func (a *flowAssembler) cleanup(now time.Time) {
	for key, state := range a.flows {
		if now.Sub(state.updatedAt) > a.limits.ttl {
			a.releaseState(state)
			delete(a.flows, key)
		}
	}
	a.lastCleanup = now
}

func (a *flowAssembler) dropFlow(key flowTuple) {
	if state := a.flows[key]; state != nil {
		a.releaseState(state)
	}
	delete(a.flows, key)
}

func (a *flowAssembler) releaseState(state *flowState) {
	a.releaseStateBuffers(state)
}

func (a *flowAssembler) releaseStateBuffers(state *flowState) {
	if state == nil {
		return
	}
	a.bufferedBytes -= len(state.buffer) + a.pendingBytes(state)
	if a.bufferedBytes < 0 {
		a.bufferedBytes = 0
	}
	clear(state.buffer)
	for index := range state.pending {
		clear(state.pending[index].payload)
	}
	state.buffer = nil
	state.pending = nil
}

func (a *flowAssembler) pendingBytes(state *flowState) int {
	total := 0
	for _, pending := range state.pending {
		total += len(pending.payload)
	}
	return total
}

func sequenceBefore(left, right uint32) bool { return int32(left-right) < 0 }
func sequenceAfter(left, right uint32) bool  { return int32(left-right) > 0 }

func decodeEthernetTCPPacket(frame []byte, destinationPort uint16) (tcpSegment, bool) {
	if len(frame) < ethernetHeaderLength {
		return tcpSegment{}, false
	}
	offset := ethernetHeaderLength
	etherType := binary.BigEndian.Uint16(frame[12:14])
	for vlanCount := 0; vlanCount < 2 && (etherType == etherTypeVLAN || etherType == etherTypeQinQ || etherType == etherTypeVLAN9100); vlanCount++ {
		if len(frame) < offset+4 {
			return tcpSegment{}, false
		}
		etherType = binary.BigEndian.Uint16(frame[offset+2 : offset+4])
		offset += 4
	}

	var sourceAddress, destinationAddress netip.Addr
	var transport []byte
	switch etherType {
	case etherTypeIPv4:
		var ok bool
		sourceAddress, destinationAddress, transport, ok = decodeIPv4TCP(frame[offset:])
		if !ok {
			return tcpSegment{}, false
		}
	case etherTypeIPv6:
		var ok bool
		sourceAddress, destinationAddress, transport, ok = decodeIPv6TCP(frame[offset:])
		if !ok {
			return tcpSegment{}, false
		}
	default:
		return tcpSegment{}, false
	}
	if len(transport) < 20 {
		return tcpSegment{}, false
	}
	sourcePort := binary.BigEndian.Uint16(transport[0:2])
	targetPort := binary.BigEndian.Uint16(transport[2:4])
	if targetPort != destinationPort {
		return tcpSegment{}, false
	}
	headerLength := int(transport[12]>>4) * 4
	if headerLength < 20 || headerLength > len(transport) {
		return tcpSegment{}, false
	}
	flags := transport[13]
	return tcpSegment{
		flow: flowTuple{
			sourceAddress:      sourceAddress.Unmap(),
			sourcePort:         sourcePort,
			destinationAddress: destinationAddress.Unmap(),
			destinationPort:    targetPort,
		},
		sequence: binary.BigEndian.Uint32(transport[4:8]),
		syn:      flags&0x02 != 0,
		fin:      flags&0x01 != 0,
		rst:      flags&0x04 != 0,
		payload:  transport[headerLength:],
	}, true
}

func decodeIPv4TCP(packet []byte) (netip.Addr, netip.Addr, []byte, bool) {
	if len(packet) < 20 || packet[0]>>4 != 4 || packet[9] != ipProtocolTCP {
		return netip.Addr{}, netip.Addr{}, nil, false
	}
	headerLength := int(packet[0]&0x0f) * 4
	totalLength := int(binary.BigEndian.Uint16(packet[2:4]))
	fragment := binary.BigEndian.Uint16(packet[6:8])
	if headerLength < 20 || totalLength < headerLength || totalLength > len(packet) || fragment&0x3fff != 0 {
		return netip.Addr{}, netip.Addr{}, nil, false
	}
	source := netip.AddrFrom4([4]byte(packet[12:16]))
	destination := netip.AddrFrom4([4]byte(packet[16:20]))
	return source, destination, packet[headerLength:totalLength], true
}

func decodeIPv6TCP(packet []byte) (netip.Addr, netip.Addr, []byte, bool) {
	if len(packet) < 40 || packet[0]>>4 != 6 {
		return netip.Addr{}, netip.Addr{}, nil, false
	}
	payloadLength := int(binary.BigEndian.Uint16(packet[4:6]))
	if 40+payloadLength > len(packet) {
		return netip.Addr{}, netip.Addr{}, nil, false
	}
	sourceArray := [16]byte(packet[8:24])
	destinationArray := [16]byte(packet[24:40])
	source := netip.AddrFrom16(sourceArray)
	destination := netip.AddrFrom16(destinationArray)
	nextHeader := packet[6]
	offset := 40
	end := 40 + payloadLength
	for extensionCount := 0; nextHeader != ipProtocolTCP && extensionCount < 8; extensionCount++ {
		if offset+2 > end {
			return netip.Addr{}, netip.Addr{}, nil, false
		}
		switch nextHeader {
		case 0, 43, 60: // Hop-by-Hop、Routing、Destination Options
			headerLength := (int(packet[offset+1]) + 1) * 8
			nextHeader = packet[offset]
			offset += headerLength
		case 51: // AH
			headerLength := (int(packet[offset+1]) + 2) * 4
			nextHeader = packet[offset]
			offset += headerLength
		case 44, 50, 59: // 分片、ESP 与 No Next Header 不进入 TCP 重组。
			return netip.Addr{}, netip.Addr{}, nil, false
		default:
			return netip.Addr{}, netip.Addr{}, nil, false
		}
		if offset > end {
			return netip.Addr{}, netip.Addr{}, nil, false
		}
	}
	if nextHeader != ipProtocolTCP || offset > end {
		return netip.Addr{}, netip.Addr{}, nil, false
	}
	return source, destination, packet[offset:end], true
}
