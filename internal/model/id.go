package model

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

var fallbackIDCounter uint64

func NewID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}

	// 理论上 crypto/rand 在正常系统里不应失败。这里不用 panic，避免一次 ID
	// 生成失败直接打断用户请求；时间戳加进程内自增足够作为降级唯一 ID。
	fallback := make([]byte, 16)
	putUint64Hex(fallback[:8], uint64(time.Now().UnixNano()))
	putUint64Hex(fallback[8:], atomic.AddUint64(&fallbackIDCounter, 1))
	return string(fallback)
}

func putUint64Hex(dst []byte, value uint64) {
	const digits = "0123456789abcdef"
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i] = digits[value&0xf]
		value >>= 4
	}
}
