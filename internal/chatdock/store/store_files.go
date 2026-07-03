package store

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// 文件和时间格式 helper 没有业务状态，单独放置可以减少 store_*.go 的噪音。

func writeRawFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

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
