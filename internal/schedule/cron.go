package schedule

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var scheduledTaskCronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

func normalizeCronSchedule(expressions []string, timezone string, now time.Time) ([]string, string, time.Time, error) {
	timezone, location, err := normalizeTimezone(timezone)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	normalized := make([]string, 0, len(expressions))
	seen := make(map[string]struct{}, len(expressions))
	for _, expression := range expressions {
		expression = strings.TrimSpace(expression)
		if expression == "" {
			continue
		}
		if _, exists := seen[expression]; exists {
			continue
		}
		if _, err := parseCronExpression(expression, timezone); err != nil {
			return nil, "", time.Time{}, err
		}
		seen[expression] = struct{}{}
		normalized = append(normalized, expression)
	}
	if len(normalized) == 0 {
		return nil, "", time.Time{}, fmt.Errorf("cron_expressions must contain at least one expression")
	}

	next, err := nextCronRun(now.In(location), normalized, timezone)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return normalized, timezone, next, nil
}

func nextCronRun(now time.Time, expressions []string, timezone string) (time.Time, error) {
	_, location, err := normalizeTimezone(timezone)
	if err != nil {
		return time.Time{}, err
	}
	now = now.In(location)

	var next time.Time
	for _, expression := range expressions {
		schedule, err := parseCronExpression(expression, timezone)
		if err != nil {
			return time.Time{}, err
		}
		candidate := schedule.Next(now)
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("cron_expressions must contain at least one expression")
	}
	return next, nil
}

func parseCronExpression(expression string, timezone string) (cron.Schedule, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("cron expression is empty")
	}
	schedule, err := scheduledTaskCronParser.Parse("CRON_TZ=" + timezone + " " + expression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expression, err)
	}
	return schedule, nil
}

func normalizeTimezone(value string) (string, *time.Location, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = configuredTimezone()
	}
	location, err := time.LoadLocation(value)
	if err != nil {
		return "", nil, fmt.Errorf("invalid timezone %q: %w", value, err)
	}
	return value, location, nil
}

func configuredTimezone() string {
	name := strings.TrimSpace(os.Getenv("CHATDOCK_TIMEZONE"))
	if name == "" {
		return "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(name); err != nil {
		return "Asia/Shanghai"
	}
	return name
}

func location() *time.Location {
	location, _ := time.LoadLocation(configuredTimezone())
	return location
}
