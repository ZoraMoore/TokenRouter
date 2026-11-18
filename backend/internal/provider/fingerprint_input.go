package provider

import (
	"maps"
	"strings"

	"github.com/google/uuid"
)

func CanonicalCodexFingerprintSeed(value any) (string, bool) {
	raw, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(raw)
	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed == uuid.Nil || trimmed != parsed.String() {
		return "", false
	}
	return trimmed, true
}

func StripCodexFingerprintSeed(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	stripped := maps.Clone(extra)
	delete(stripped, CodexFingerprintSeedExtraKey)
	return stripped
}

func CodexFingerprintSeed(extra map[string]any) (string, bool) {
	if extra == nil {
		return "", false
	}
	return CanonicalCodexFingerprintSeed(extra[CodexFingerprintSeedExtraKey])
}

func PrepareCodexFingerprintExtraForCreate(platform, providerType string, extra map[string]any, newSeed func() string) map[string]any {
	prepared := StripCodexFingerprintSeed(extra)
	if platform != PlatformOpenAI || (providerType != ProviderTypeOAuth && providerType != ProviderTypeSetupToken) || !CodexFingerprintModeRequiresSeed(CodexFingerprintModeFromExtra(prepared)) {
		return prepared
	}
	if prepared == nil {
		prepared = make(map[string]any, 1)
	}
	prepared[CodexFingerprintSeedExtraKey] = newSeed()
	return prepared
}

func PrepareCodexFingerprintExtraForUpdate(provider *Record, extra map[string]any, newSeed func() string) map[string]any {
	prepared := StripCodexFingerprintSeed(extra)
	if provider == nil || !provider.IsOpenAIOAuthLike() {
		return prepared
	}
	if seed, ok := CodexFingerprintSeed(provider.Extra); ok {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[CodexFingerprintSeedExtraKey] = seed
		return prepared
	}
	if CodexFingerprintModeRequiresSeed(CodexFingerprintModeFromExtra(prepared)) {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[CodexFingerprintSeedExtraKey] = newSeed()
	}
	return prepared
}

func SanitizedCodexFingerprintExtraUpdates(updates map[string]any) map[string]any {
	if updates == nil {
		return nil
	}
	sanitized := maps.Clone(updates)
	delete(sanitized, CodexFingerprintSeedExtraKey)
	return sanitized
}
