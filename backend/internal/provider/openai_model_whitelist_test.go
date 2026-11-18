package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/settings"
)

// modelWhitelistStore 可暂停一次读取，模拟两个节点共用设置表。
type modelWhitelistStore struct {
	mu          sync.Mutex
	stored      *string
	readErr     error
	reads       int
	readStarted chan struct{}
	resumeRead  chan struct{}
}

func (s *modelWhitelistStore) GetValue(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	s.reads++
	raw, err := s.stored, s.readErr
	started, release := s.readStarted, s.resumeRead
	s.readStarted = nil
	s.mu.Unlock()
	if key != SettingKeyOpenAIOAuthModelWhitelist {
		return "", fmt.Errorf("unexpected key %s", key)
	}
	if started != nil {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err != nil {
		return "", err
	}
	if raw == nil {
		return "", settings.ErrSettingNotFound
	}
	return *raw, nil
}

func (s *modelWhitelistStore) Set(_ context.Context, key, raw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != SettingKeyOpenAIOAuthModelWhitelist {
		return fmt.Errorf("unexpected key %s", key)
	}
	s.stored = &raw
	return nil
}

func TestOpenAIOAuthModelWhitelistDefaultsAndExactMatches(t *testing.T) {
	shared := &modelWhitelistStore{}
	node := NewRuntimeSettings(shared, settings.ErrSettingNotFound)
	for _, reader := range []*RuntimeSettings{nil, NewRuntimeSettings(nil, nil), node} {
		initial, err := reader.GetOpenAIOAuthModelWhitelist(t.Context())
		require.NoError(t, err)
		require.Equal(t, openAIDefaults.Models, initial.Models)
		require.False(t, initial.Configured)
		initial.Models[0] = "changed-by-caller"
		policy, err := reader.OpenAIOAuthModelWhitelistSnapshot(t.Context())
		require.NoError(t, err)
		require.True(t, policy.Allows("historical-model"))
		require.Empty(t, policy.Models())
		next, err := reader.GetOpenAIOAuthModelWhitelist(t.Context())
		require.NoError(t, err)
		require.Equal(t, openAIDefaults.Models, next.Models)
		require.NotEqual(t, initial.Models, next.Models)
	}
	input := []string{" gpt-Custom ", "gpt-Custom", "vendor/fine.tuned-model:V2_test"}
	saved, err := node.SetOpenAIOAuthModelWhitelist(t.Context(), input)
	require.NoError(t, err)
	require.True(t, saved.Configured)
	require.Equal(t, []string{"gpt-Custom", "vendor/fine.tuned-model:V2_test"}, saved.Models)
	input[2] = "changed-input"
	saved.Models[0] = "changed-output"
	policy, err := node.OpenAIOAuthModelWhitelistSnapshot(t.Context())
	require.NoError(t, err)
	require.True(t, policy.Configured())
	require.True(t, policy.Allows("gpt-Custom"))
	require.True(t, policy.Allows("vendor/fine.tuned-model:V2_test"))
	for _, id := range []string{"gpt-custom", "gpt-Custom-latest", "fine.tuned-model:V2_test", "changed-output", "changed-input", ""} {
		require.False(t, policy.Allows(id), id)
	}
	snapshotModels := policy.Models()
	snapshotModels[1] = "changed-snapshot"
	next, err := node.GetOpenAIOAuthModelWhitelist(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-Custom", "vendor/fine.tuned-model:V2_test"}, next.Models)
	require.Equal(t, 1, shared.reads)
}

func TestOpenAIOAuthModelWhitelistValidation(t *testing.T) {
	node := NewRuntimeSettings(&modelWhitelistStore{}, settings.ErrSettingNotFound)
	for _, input := range [][]string{
		nil,
		{},
		{""},
		{"  "},
		{"-starts-with-punctuation"},
		{"*"},
		{"gpt-*"},
		{"bad model"},
		{"cut\nmodel"},
		{"\tgpt-name"},
		{"gpt-name\r"},
		{"name\x00"},
		{"https://example.test"},
		{"gpt-模型"},
		{strings.Repeat("a", 256)},
		make([]string, 257),
	} {
		_, err := node.SetOpenAIOAuthModelWhitelist(t.Context(), input)
		require.Error(t, err)
	}
	maximum := make([]string, 256)
	for i := range maximum {
		maximum[i] = fmt.Sprintf("gpt-%d", i)
	}
	maximum[0] = strings.Repeat("a", 255)
	saved, err := node.SetOpenAIOAuthModelWhitelist(t.Context(), maximum)
	require.NoError(t, err)
	require.Equal(t, maximum, saved.Models)
}

func TestOpenAIOAuthModelWhitelistRefreshesAcrossInstances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &modelWhitelistStore{}
		writer := NewRuntimeSettings(store, settings.ErrSettingNotFound)
		reader := NewRuntimeSettings(store, settings.ErrSettingNotFound)
		_, err := writer.SetOpenAIOAuthModelWhitelist(t.Context(), []string{"first"})
		require.NoError(t, err)
		first, err := reader.OpenAIOAuthModelWhitelistSnapshot(t.Context())
		require.NoError(t, err)
		require.True(t, first.Allows("first"))

		_, err = writer.SetOpenAIOAuthModelWhitelist(t.Context(), []string{"second"})
		require.NoError(t, err)
		time.Sleep(5*time.Second - time.Nanosecond)
		cached, err := reader.OpenAIOAuthModelWhitelistSnapshot(t.Context())
		require.NoError(t, err)
		require.True(t, cached.Allows("first"))
		time.Sleep(time.Nanosecond)
		current, err := reader.OpenAIOAuthModelWhitelistSnapshot(t.Context())
		require.NoError(t, err)
		require.False(t, current.Allows("first"))
		require.True(t, current.Allows("second"))
		require.False(t, first.Allows("second"))
	})
}

func TestOpenAIOAuthModelWhitelistReadFailureRejectsModelsAndRecovers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &modelWhitelistStore{}
		node := NewRuntimeSettings(store, settings.ErrSettingNotFound)
		_, err := node.SetOpenAIOAuthModelWhitelist(t.Context(), []string{"gpt-allowed"})
		require.NoError(t, err)
		time.Sleep(5 * time.Second)
		store.readErr = errors.New("database unavailable")
		for range 2 {
			policy, err := node.OpenAIOAuthModelWhitelistSnapshot(t.Context())
			require.ErrorIs(t, err, store.readErr)
			require.True(t, policy.Configured())
			require.False(t, policy.Allows("gpt-unlisted"))
			require.False(t, policy.Allows("gpt-allowed"))
		}
		require.Equal(t, 1, store.reads)
		store.readErr = nil
		for _, invalid := range []string{"", "{}", "null", "[]", `["gpt-*"]`, `["valid",432]`} {
			require.NoError(t, store.Set(t.Context(), SettingKeyOpenAIOAuthModelWhitelist, invalid))
			time.Sleep(time.Second)
			policy, err := node.OpenAIOAuthModelWhitelistSnapshot(t.Context())
			require.Error(t, err)
			require.False(t, policy.Allows("gpt-unlisted"))
		}
		_, err = node.SetOpenAIOAuthModelWhitelist(t.Context(), []string{"gpt-recovered"})
		require.NoError(t, err)
		policy, err := node.OpenAIOAuthModelWhitelistSnapshot(t.Context())
		require.NoError(t, err)
		require.True(t, policy.Allows("gpt-recovered"))
	})
}

func TestOpenAIOAuthModelWhitelistInflightReadCannotReplaceSavedPolicy(t *testing.T) {
	store := &modelWhitelistStore{readStarted: make(chan struct{}), resumeRead: make(chan struct{})}
	started := store.readStarted
	node := NewRuntimeSettings(store, settings.ErrSettingNotFound)
	readDone := make(chan error, 1)
	go func() {
		_, err := node.GetOpenAIOAuthModelWhitelist(t.Context())
		readDone <- err
	}()
	<-started
	written := make(chan error, 1)
	go func() {
		_, err := node.SetOpenAIOAuthModelWhitelist(t.Context(), []string{"saved"})
		written <- err
	}()
	close(store.resumeRead)
	require.NoError(t, <-readDone)
	require.NoError(t, <-written)
	policy, err := node.OpenAIOAuthModelWhitelistSnapshot(t.Context())
	require.NoError(t, err)
	require.True(t, policy.Allows("saved"))
	require.False(t, policy.Allows("other"))
}
