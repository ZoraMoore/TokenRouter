package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

func RelayRealtime(ctx context.Context, client, conn upstream.FrameConn) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	var audioObserved atomic.Bool

	// 上游到客户端。
	go func() {
		for {
			_, msg, readErr := conn.ReadFrame(ctx)
			if readErr != nil {
				errCh <- readErr
				return
			}
			if GrokRealtimeEventHasAudio(msg) {
				audioObserved.Store(true)
			}
			if writeErr := client.WriteFrame(ctx, upstream.FrameText, msg); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
	}()

	// 客户端到上游，仅接受 JSON 事件。
	go func() {
		for {
			kind, msg, readErr := client.ReadFrame(ctx)
			if readErr != nil {
				errCh <- readErr
				return
			}
			if kind != upstream.FrameText && kind != upstream.FrameBinary {
				continue
			}
			if GrokRealtimeEventHasAudio(msg) {
				audioObserved.Store(true)
			}
			var raw json.RawMessage
			if unmarshalErr := json.Unmarshal(msg, &raw); unmarshalErr != nil {
				errCh <- fmt.Errorf("invalid realtime event: %w", unmarshalErr)
				return
			}
			if writeErr := conn.WriteFrame(ctx, upstream.FrameText, raw); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
	}()

	return AwaitGrokRealtimeAudioObserved(errCh, &audioObserved)
}
