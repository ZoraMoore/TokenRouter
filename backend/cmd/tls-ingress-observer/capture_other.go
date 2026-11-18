//go:build !linux

package main

import (
	"context"
	"errors"
)

type packetCapture struct{}

func openPacketCapture(string, int) (*packetCapture, string, error) {
	return nil, "", errors.New("authenticated ingress observer requires Linux AF_PACKET")
}

func (*packetCapture) Read(context.Context, []byte) (int, error) {
	return 0, errors.New("authenticated ingress observer requires Linux AF_PACKET")
}

func (*packetCapture) Close() error { return nil }

func (*packetCapture) ReceiveBufferBytes() int { return 0 }

func (*packetCapture) Statistics() (uint32, uint32, error) {
	return 0, 0, errors.New("authenticated ingress observer requires Linux AF_PACKET")
}
