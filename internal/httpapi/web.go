package httpapi

import (
	"bytes"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	webui "chatdock/web"
)

func (a *Server) webHandler() http.Handler {
	var webFS fs.FS
	if dir := strings.TrimSpace(a.cfg.WebDir); dir != "" {
		webFS = os.DirFS(dir)
	} else {
		embedded, err := fs.Sub(webui.Dist, "dist")
		if err != nil {
			log.Printf("embedded web dist unavailable: %v", err)
			return http.NotFoundHandler()
		}
		webFS = embedded
	}
	return spaFileServer(webFS)
}

func spaFileServer(webFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 前端路由只接管页面路径；API/MCP 路由缺失时必须保持后端 404，不能回落成 index.html。
		if isBackendRoute(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if file, err := webFS.Open(name); err == nil {
			stat, statErr := file.Stat()
			_ = file.Close()
			if statErr == nil && !stat.IsDir() {
				setWebCacheHeader(w, name)
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}

		index, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		setWebCacheHeader(w, "index.html")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
}

func setWebCacheHeader(w http.ResponseWriter, name string) {
	cleaned := strings.TrimPrefix(path.Clean("/"+name), "/")
	switch {
	case cleaned == "index.html" || strings.HasSuffix(cleaned, ".html"):
		w.Header().Set("Cache-Control", "no-store")
	case strings.HasPrefix(cleaned, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		w.Header().Set("Cache-Control", "no-cache")
	}
}

func isBackendRoute(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") || requestPath == "/mcp" || strings.HasPrefix(requestPath, "/mcp/")
}

func RemoveLegacyRuntimeArtifacts(repoDir string) error {
	for _, name := range []string{"chatdock", "bin"} {
		path := repoDir + string(os.PathSeparator) + name
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
