package llm

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxAutoModelRetryDelay = 5 * time.Second

// IsRetryableModelError 只判断故障是否属于可恢复类型，不决定是否适合在当前模型上立即重试。
// 例如 Retry-After 很长的 429 仍可切换备用模型，但不应让交互请求原地等待。
func IsRetryableModelError(err error) bool {
	// 上下文溢出不应在同一模型上原地重试，但备用模型可能有更大的上下文窗口。
	// 因此它仍属于可通过 fallback 恢复的错误。
	if IsContextTooLargeModelError(err) {
		return true
	}
	_, retryable := modelRetryAfter(err)
	return retryable
}

// ModelRetryDelay 判断一次模型调用失败是否属于瞬时故障，并返回下一次重试前的等待时间。
// HTTP 参数、鉴权和协议错误不会进入重试，避免把确定性失败放大成多次无效请求。
func ModelRetryDelay(err error, fallback time.Duration) (time.Duration, bool) {
	if fallback < 0 {
		fallback = 0
	}

	retryAfter, retryable := modelRetryAfter(err)
	if !retryable {
		return 0, false
	}
	if retryAfter > maxAutoModelRetryDelay {
		return 0, false
	}
	if retryAfter > fallback {
		fallback = retryAfter
	}
	return fallback, true
}

func modelRetryAfter(err error) (time.Duration, bool) {
	if err == nil || errors.Is(err, context.Canceled) {
		return 0, false
	}
	if IsContextTooLargeModelError(err) {
		return 0, false
	}

	var apiErr *modelAPIResponseError
	// statusCode=0 代表 HTTP 200 后由 SSE error chunk 报出的带内错误。
	// 这类错误仍需继续检查底层传输错误文本，不能被 HTTP 状态判断提前短路。
	if errors.As(err, &apiErr) && apiErr.statusCode != 0 {
		return apiErr.retryAfter, retryableModelStatus(apiErr.statusCode)
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT) {
		return 0, true
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return 0, true
	}

	// 少数 OpenAI 兼容中转会把底层网络错误压平成普通字符串，无法保留可 errors.Is 的错误链。
	// 这里只匹配明确的传输层故障，不使用宽泛的“失败”“超时”等关键词。
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"unexpected eof",
		"connection reset by peer",
		"connection refused",
		"broken pipe",
		"server closed idle connection",
		"tls handshake timeout",
		"i/o timeout",
	} {
		if strings.Contains(message, fragment) {
			return 0, true
		}
	}
	return 0, false
}

func retryableModelStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func parseModelRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}
