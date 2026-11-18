package provider

import (
	"encoding/json"
	"strings"
)

func OllamaCloudUsageStateFromProvider(provider *Record) *OllamaCloudUsageState {
	state := &OllamaCloudUsageState{}
	if provider == nil {
		return state
	}
	state.ProviderID = provider.ID
	state.Eligible = IsOllamaCloudUsageProvider(provider)
	if !state.Eligible {
		return state
	}
	state.Configured = OllamaCloudUsageConfigured(provider)
	state.AutoRefreshEnabled = state.Configured && OllamaCloudUsageAutoRefreshEnabled(provider)
	state.Snapshot = DecodeOllamaCloudUsageSnapshot(provider.Extra)
	return state
}

func OllamaCloudUsageConfigured(provider *Record) bool {
	if provider == nil || provider.Extra == nil {
		return false
	}
	value, ok := provider.Extra[OllamaCloudUsageSessionExtraKey].(string)
	return ok && strings.TrimSpace(value) != ""
}

func OllamaCloudUsageAutoRefreshEnabled(provider *Record) bool {
	if provider == nil || provider.Extra == nil {
		return false
	}
	enabled, ok := provider.Extra[OllamaCloudUsageAutoRefreshExtraKey].(bool)
	return ok && enabled
}

func DecodeOllamaCloudUsageSnapshot(extra map[string]any) *OllamaCloudUsageSnapshot {
	if extra == nil {
		return nil
	}
	value, ok := extra[OllamaCloudUsageSnapshotExtraKey]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var snapshot OllamaCloudUsageSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil
	}
	if snapshot.Status != OllamaCloudUsageStatusOK && snapshot.Status != OllamaCloudUsageStatusUnauthorized && snapshot.Status != OllamaCloudUsageStatusFailed {
		return nil
	}
	return &snapshot
}
