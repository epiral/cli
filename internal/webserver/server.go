// Package webserver 提供 Web 管理面板的 HTTP 服务。
// 内嵌 React 构建产物，同时提供 REST API 和 SSE 日志流。
package webserver

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/epiral/cli/internal/config"
	"github.com/epiral/cli/internal/daemon"
	"github.com/epiral/cli/internal/logger"
)

//go:embed all:dist
var distFS embed.FS

// Server 是 Web 管理面板的 HTTP 服务
type Server struct {
	port       int
	store      *config.Store
	logBuf     *logger.RingBuffer
	manager    *daemon.Manager
	httpServer *http.Server
	appCtx     context.Context // 用于 restart daemon
}

// New 创建 Web 服务
func New(port int, store *config.Store, logBuf *logger.RingBuffer, manager *daemon.Manager, appCtx context.Context) *Server {
	return &Server{
		port:    port,
		store:   store,
		logBuf:  logBuf,
		manager: manager,
		appCtx:  appCtx,
	}
}

// Start 启动 HTTP 服务（阻塞）
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("GET /api/status", s.handleGetStatus)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("GET /api/logs", s.handleGetLogs)
	mux.HandleFunc("GET /api/logs/stream", s.handleLogStream)

	// 静态文件（React 构建产物）
	distContent, err := fs.Sub(distFS, "dist")
	if err != nil {
		return fmt.Errorf("加载静态资源失败: %w", err)
	}
	fileServer := http.FileServer(http.FS(distContent))
	mux.Handle("/", spaHandler(fileServer, distContent))

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("[Web] 管理面板: http://localhost:%d", s.port)
	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// --- API Handlers ---

func (s *Server) handleGetStatus(w http.ResponseWriter, _ *http.Request) {
	status := s.manager.Status()
	cfg := s.store.Get()
	writeJSON(w, map[string]any{
		"daemon":     status,
		"configured": cfg.IsConfigured(),
		"configPath": s.store.Path(),
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.store.Get()
	writeJSON(w, cfg)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取请求失败")
		return
	}

	var cfg config.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("解析配置失败: %v", err))
		return
	}

	// 确保默认值
	if cfg.Browser.Port == 0 {
		cfg.Browser.Port = 19824
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = s.port
	}

	if err := s.store.Update(&cfg); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	log.Printf("[Web] 配置已更新，重启 Daemon...")

	// 后台重启 daemon
	go s.manager.Restart(s.appCtx)

	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleGetLogs(w http.ResponseWriter, _ *http.Request) {
	entries := s.logBuf.All()
	writeJSON(w, map[string]any{"entries": entries})
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := s.logBuf.Subscribe()
	defer s.logBuf.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(entry)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// --- 辅助函数 ---

// spaHandler 处理 SPA 路由：静态文件存在则返回，否则返回 index.html
func spaHandler(fileServer http.Handler, fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else {
			path = strings.TrimPrefix(path, "/")
		}

		// 文件存在则直接返回
		if _, err := fs.Stat(fsys, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// 如果是 API 路径，不要 fallback
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: 返回 index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// withCORS 添加 CORS 头（开发时 vite dev server 跨域需要）
func withCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// DevProxy 检查是否启用了开发模式（环境变量）
func DevProxy() bool {
	return os.Getenv("EPIRAL_DEV") == "1"
}
