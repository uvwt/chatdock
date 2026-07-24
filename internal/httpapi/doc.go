// Package httpapi 负责 HTTP 路由、Server 生命周期和业务编排。
//
// 这个包刻意只保留“把请求串起来”的代码：路由、鉴权、会话调度、上传入口、
// 以及 store / LLM / MCP 的组合调用。稳定 DTO、模型请求和 MCP 协议实现不要继续塞回这里。
package httpapi
