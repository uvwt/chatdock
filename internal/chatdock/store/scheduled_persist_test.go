package store

import (
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestNextDailyRunUsesConfiguredScheduleTimezone(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	now := time.Date(2026, 7, 6, 2, 50, 0, 0, time.UTC) // 10:50 in Asia/Shanghai.
	got := nextDailyRun(now, "20:30")
	loc := scheduleLocation()
	if got.In(loc).Format("2006-01-02 15:04") != "2026-07-06 20:30" {
		t.Fatalf("next daily run = %s, want 2026-07-06 20:30 in %s", got.In(loc), loc)
	}
	if got.UTC().Format(time.RFC3339) != "2026-07-06T12:30:00Z" {
		t.Fatalf("next daily run UTC = %s, want 2026-07-06T12:30:00Z", got.UTC().Format(time.RFC3339))
	}
}

func TestRepairDailyNextRunFixesOldUTCStoredTime(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	task := model.ScheduledTask{
		ScheduleType: scheduleTypeDaily,
		TimeOfDay:    "20:30",
		NextRunAt:    time.Date(2026, 7, 6, 20, 30, 0, 0, time.UTC), // Old bug: 20:30 UTC, 04:30 local next day.
	}
	now := time.Date(2026, 7, 6, 2, 50, 0, 0, time.UTC)
	if !repairScheduledTaskNextRun(&task, now) {
		t.Fatal("expected repair for UTC-stored daily next_run_at")
	}
	loc := scheduleLocation()
	if task.NextRunAt.In(loc).Format("2006-01-02 15:04") != "2026-07-06 20:30" {
		t.Fatalf("repaired next_run_at = %s, want local 2026-07-06 20:30", task.NextRunAt.In(loc))
	}
}

func TestParseTaskTimeUsesScheduleTimezoneForLocalInput(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	got, err := parseTaskTime("2026-07-06T09:30")
	if err != nil {
		t.Fatal(err)
	}
	loc := scheduleLocation()
	if got.In(loc).Format("2006-01-02 15:04") != "2026-07-06 09:30" {
		t.Fatalf("parsed local run time = %s", got.In(loc))
	}
	if got.UTC().Format(time.RFC3339) != "2026-07-06T01:30:00Z" {
		t.Fatalf("parsed UTC run time = %s, want 2026-07-06T01:30:00Z", got.UTC().Format(time.RFC3339))
	}
}
