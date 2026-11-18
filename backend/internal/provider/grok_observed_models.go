package provider

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const (
	GrokObservedModelsExtraKey = "grok_observed_models"
	GrokObservedModelsTTL      = 6 * time.Hour
	GrokObservedModelsTimeout  = 15 * time.Second
)

type GrokObservedModelsSnapshot struct {
	Models    []string `json:"models"`
	FetchedAt string   `json:"fetched_at"`
	Source    string   `json:"source,omitempty"`
}
type GrokModelsOptions struct {
	Runtime     *ProbeRuntime
	Available   func() bool
	Token       func(context.Context, *Record) (string, error)
	Fetch       func(context.Context, *Record, string) ([]string, error)
	UpdateExtra func(context.Context, int64, map[string]any) error
	Debug       func(string, ...any)
}

func ScheduleGrokObservedModels(o GrokModelsOptions, provider *Record) {
	if provider == nil || !provider.IsGrokOAuth() || !o.Available() {
		return
	}
	acc := CloneRecord(provider)
	o.Runtime.Schedule("models:"+strconv.FormatInt(acc.ID, 10), GrokObservedModelsTimeout, func(ctx context.Context) {
		if err := SyncGrokObservedModels(ctx, o, acc); err != nil {
			o.Debug("grok_observed_models_sync_failed", "provider_id", acc.ID, "error", err)
		}
	})
}

func SyncGrokObservedModels(ctx context.Context, o GrokModelsOptions, provider *Record) error {
	if provider == nil {
		return nil
	}
	if snap := ParseGrokObservedModels(provider.Extra); snap != nil {
		if t, err := time.Parse(time.RFC3339, snap.FetchedAt); err == nil && time.Since(t) < GrokObservedModelsTTL {
			return nil
		}
	}
	token := strings.TrimSpace(provider.GetGrokAccessToken())
	if token == "" && o.Token != nil {
		if at, err := o.Token(ctx, provider); err == nil {
			token = strings.TrimSpace(at)
		}
	}
	if token == "" {
		return nil
	}
	ids, err := o.Fetch(ctx, provider, token)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	snap := GrokObservedModelsSnapshot{Models: ids, FetchedAt: time.Now().UTC().Format(time.RFC3339), Source: "upstream_v1_models"}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return o.UpdateExtra(ctx, provider.ID, map[string]any{GrokObservedModelsExtraKey: asMap})
}

func ParseGrokObservedModels(extra map[string]any) *GrokObservedModelsSnapshot {
	if extra == nil {
		return nil
	}
	raw, ok := extra[GrokObservedModelsExtraKey]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snap GrokObservedModelsSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil
	}
	if len(snap.Models) == 0 {
		return nil
	}
	return &snap
}
