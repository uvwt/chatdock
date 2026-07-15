package chatdock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"chatdock/internal/chatdock/mcp"
)

const (
	builtinToolLoadImageURL = "chatdock_image_url_load"
	builtinToolServerImages = "chatdock"

	maxImageURLBytes = 20 << 20
)

func builtinImageTools() []mcp.MCPTool {
	return []mcp.MCPTool{
		{
			Server:      builtinToolServerImages,
			Name:        "image_url_load",
			FullName:    builtinToolLoadImageURL,
			Title:       "加载图片链接",
			Description: "加载并校验一张 http/https 图片链接，将图片作为模型可见的视觉输入发送给大模型；默认只传 image_url 内容块，不把图片 base64 塞进普通文本上下文。适合用户给出图片超链接并要求看图、识图、分析图片。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"url":    map[string]any{"type": "string", "description": "图片的 http 或 https URL。必须是公网可访问的图片链接。"},
				"prompt": map[string]any{"type": "string", "description": "随图片一起发送给模型的说明或问题。为空时使用默认说明。"},
				"detail": map[string]any{"type": "string", "enum": []string{"auto", "low", "high"}, "description": "模型视觉细节级别，默认 auto。"},
			}, "required": []string{"url"}},
		},
	}
}

func isBuiltinImageTool(name string) bool {
	return name == builtinToolLoadImageURL
}

func (a *App) callBuiltinImageTool(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case builtinToolLoadImageURL:
		return loadImageURLForModel(ctx, args)
	default:
		return nil, fmt.Errorf("unknown builtin image tool: %s", name)
	}
}

func loadImageURLForModel(ctx context.Context, args map[string]any) (map[string]any, error) {
	rawURL, err := requiredStringArg(args, "url")
	if err != nil {
		return nil, err
	}
	parsed, err := validatePublicHTTPImageURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	prompt := strings.TrimSpace(stringArg(args, "prompt"))
	if prompt == "" {
		prompt = "请分析这张图片。"
	}
	detail := strings.ToLower(strings.TrimSpace(stringArg(args, "detail")))
	if detail == "" {
		detail = "auto"
	}
	if detail != "auto" && detail != "low" && detail != "high" {
		return nil, fmt.Errorf("detail must be one of: auto, low, high")
	}

	meta, err := probeImageURL(ctx, parsed.String())
	if err != nil {
		return nil, err
	}
	imageURL := map[string]any{"url": parsed.String()}
	if detail != "auto" {
		imageURL["detail"] = detail
	}
	modelContent := []map[string]any{
		{"type": "text", "text": "ChatDock 已加载用户提供的图片链接，并将下方图片作为视觉输入发送给你。\n\n" + prompt},
		{"type": "image_url", "image_url": imageURL},
	}
	return map[string]any{
		"url":                        parsed.String(),
		"mime_type":                  meta.MIMEType,
		"size_bytes":                 meta.SizeBytes,
		"detail":                     detail,
		"model_delivery":             "image_url",
		"status":                     "图片链接已校验，并会作为视觉输入发送给模型。",
		"_chatdock_model_content":    modelContent,
		"_chatdock_model_role":       "user",
		"_chatdock_model_content_ok": true,
	}, nil
}

type imageURLMeta struct {
	MIMEType  string
	SizeBytes int64
}

func validatePublicHTTPImageURL(ctx context.Context, raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("url must use http or https")
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("url host is required")
	}
	if err := rejectPrivateHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func probeImageURL(ctx context.Context, rawURL string) (imageURLMeta, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout:   25 * time.Second,
		Transport: publicImageTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL == nil {
				return fmt.Errorf("redirect url is empty")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirect url must use http or https")
			}
			return rejectPrivateHost(req.Context(), req.URL.Hostname())
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return imageURLMeta{}, err
	}
	req.Header.Set("User-Agent", "ChatDock/1.0 image-url-loader")
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp,image/gif,image/*;q=0.8,*/*;q=0.1")
	resp, err := client.Do(req)
	if err != nil {
		return imageURLMeta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return imageURLMeta{}, fmt.Errorf("image url returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxImageURLBytes {
		return imageURLMeta{}, fmt.Errorf("image is too large: %d bytes, max %d bytes", resp.ContentLength, maxImageURLBytes)
	}

	limited := io.LimitReader(resp.Body, maxImageURLBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return imageURLMeta{}, err
	}
	if len(raw) == 0 {
		return imageURLMeta{}, fmt.Errorf("image url returned empty body")
	}
	if len(raw) > maxImageURLBytes {
		return imageURLMeta{}, fmt.Errorf("image is too large: exceeds %d bytes", maxImageURLBytes)
	}
	mimeType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(raw)
	}
	mimeType = strings.ToLower(mimeType)
	if !isSupportedVisionImageMIME(mimeType) {
		return imageURLMeta{}, fmt.Errorf("url content is not a supported image type: %s", mimeType)
	}
	return imageURLMeta{MIMEType: mimeType, SizeBytes: int64(len(raw))}, nil
}

func publicImageTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolvePublicHostIPs(ctx, host)
			if err != nil {
				return nil, err
			}
			var dialErrors []error
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				dialErrors = append(dialErrors, dialErr)
			}
			return nil, errors.Join(dialErrors...)
		},
	}
}

func resolvePublicHostIPs(parent context.Context, host string) ([]net.IP, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("url host is required")
	}
	lower := strings.ToLower(strings.TrimSuffix(host, "."))
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return nil, fmt.Errorf("private or localhost image URLs are not allowed")
	}
	if ip := net.ParseIP(lower); ip != nil {
		if isPrivateOrLocalIP(ip) {
			return nil, fmt.Errorf("private or localhost image URLs are not allowed")
		}
		return []net.IP{ip}, nil
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", lower)
	if err != nil {
		// Go 在不同系统解析器上可能返回不包装 context.Canceled 的
		// *net.DNSError（例如 "operation was canceled"）。对外统一返回
		// context 语义，避免调用方和跨平台测试依赖错误字符串。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("url host has no DNS records")
	}
	for _, ip := range ips {
		if isPrivateOrLocalIP(ip) {
			return nil, fmt.Errorf("private or localhost image URLs are not allowed")
		}
	}
	return ips, nil
}

func rejectPrivateHost(parent context.Context, host string) error {
	_, err := resolvePublicHostIPs(parent, host)
	return err
}

var blockedImageIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),  // 运营商共享地址，常用于内部网络。
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF 协议分配。
	netip.MustParsePrefix("192.0.2.0/24"),   // 文档地址。
	netip.MustParsePrefix("192.88.99.0/24"), // 已弃用的 6to4 中继。
	netip.MustParsePrefix("198.18.0.0/15"),  // 网络设备基准测试。
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isPrivateOrLocalIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	for _, prefix := range blockedImageIPPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func isSupportedVisionImageMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png", "image/jpeg", "image/jpg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}
