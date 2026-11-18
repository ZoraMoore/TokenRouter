package bootstrap

import (
	"fmt"
	"log"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
)

// InitTimezone 在进程引导时设置 time.Local，后续装配使用该时区构造 Calendar。
func InitTimezone(name string) error {
	if name == "" {
		name = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("invalid timezone %q: %w", name, err)
	}
	time.Local = location
	log.Printf("Timezone initialized: %s (UTC offset: %s)", name, timezone.NewCalendar(location).UTCOffset(time.Now()))
	return nil
}
