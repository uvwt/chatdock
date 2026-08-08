package store

import (
	"encoding/json"
	"time"
)

// 文件和时间格式 helper 没有业务状态，单独放置可以减少 store_*.go 的噪音。

func prettyJSON(content string) (string, error) {
	var value any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}

func formatDBTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.Format(time.RFC3339Nano)
}

func parseDBTime(value string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.Local()
	}
	return time.Now()
}
