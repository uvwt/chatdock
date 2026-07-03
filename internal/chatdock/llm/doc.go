// Package llm 封装模型供应商请求、流式解析、上下文裁剪和工具调用消息转换。
//
// 它只依赖共享 model 和 MCP 工具描述，不反向依赖 HTTP App 或存储层。
package llm
