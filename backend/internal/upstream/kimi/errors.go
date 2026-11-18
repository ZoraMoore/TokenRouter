package kimi

import "strings"

const ConcurrentRequestLimitMessage = "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."

func IsConcurrencyLimitMessage(message string) bool {
	return strings.TrimSpace(message) == ConcurrentRequestLimitMessage
}
