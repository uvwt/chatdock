// Package store 负责 ChatDock 的持久化状态和基于持久化状态的派生查询。
//
// 这里直接暴露一个简单的 Store 类型，不引入 repository/service/interface 三件套。
// HTTP handler 和模型/MCP 编排留在 httpapi，避免存储层反向依赖 Server。
package store
