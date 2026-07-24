package schedule

import (
	"testing"
	"time"
)

func TestNextCronRunUsesTaskTimezoneAndEarliestExpression(t *testing.T) {
	now := time.Date(2026, 7, 6, 2, 50, 0, 0, time.UTC) // 10:50 in Asia/Shanghai.
	got, err := nextCronRun(now, []string{"30 20 * * *", "0 12 * * *"}, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	if got.In(location).Format("2006-01-02 15:04") != "2026-07-06 12:00" {
		t.Fatalf("next cron run = %s, want 2026-07-06 12:00 in %s", got.In(location), location)
	}
	if got.UTC().Format(time.RFC3339) != "2026-07-06T04:00:00Z" {
		t.Fatalf("next cron run UTC = %s, want 2026-07-06T04:00:00Z", got.UTC().Format(time.RFC3339))
	}
}

func TestNextCronRunKeepsWallClockAcrossDSTChanges(t *testing.T) {
	location, _ := time.LoadLocation("America/New_York")
	cases := map[string]struct {
		now        time.Time
		wantDate   string
		wantOffset string
	}{
		"spring forward": {now: time.Date(2026, 3, 7, 10, 0, 0, 0, location), wantDate: "2026-03-08 09:00", wantOffset: "-04:00"},
		"fall back":      {now: time.Date(2026, 10, 31, 10, 0, 0, 0, location), wantDate: "2026-11-01 09:00", wantOffset: "-05:00"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := nextCronRun(tc.now, []string{"0 9 * * *"}, "America/New_York")
			if err != nil {
				t.Fatal(err)
			}
			if got.In(location).Format("2006-01-02 15:04") != tc.wantDate {
				t.Fatalf("next cron run = %s, want %s", got.In(location), tc.wantDate)
			}
			if got.Format("-07:00") != tc.wantOffset {
				t.Fatalf("next cron run offset = %s, want %s", got.Format("-07:00"), tc.wantOffset)
			}
		})
	}
}

func TestNormalizeCronScheduleRejectsInvalidConfiguration(t *testing.T) {
	if _, _, _, err := normalizeCronSchedule([]string{"not a cron"}, "Asia/Shanghai", time.Now()); err == nil {
		t.Fatal("invalid cron expression should fail")
	}
	if _, _, _, err := normalizeCronSchedule([]string{"0 9 * * *"}, "Mars/Base", time.Now()); err == nil {
		t.Fatal("invalid timezone should fail")
	}
}

func TestParseTaskTimeUsesScheduleTimezoneForLocalInput(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	got, err := parseTaskTime("2026-07-06T09:30")
	if err != nil {
		t.Fatal(err)
	}
	loc := location()
	if got.In(loc).Format("2006-01-02 15:04") != "2026-07-06 09:30" {
		t.Fatalf("parsed local run time = %s", got.In(loc))
	}
	if got.UTC().Format(time.RFC3339) != "2026-07-06T01:30:00Z" {
		t.Fatalf("parsed UTC run time = %s, want 2026-07-06T01:30:00Z", got.UTC().Format(time.RFC3339))
	}
}
