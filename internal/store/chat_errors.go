package store

import (
	"errors"
	"fmt"
)

// ErrInvalidChatRequest 标识用户可修正的聊天准备错误，例如空消息、
// 不存在的模型选择或不允许重新生成的会话状态。持久化和数据库错误不能包装成它。
var ErrInvalidChatRequest = errors.New("invalid chat request")

func invalidChatRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidChatRequest, fmt.Sprintf(format, args...))
}
