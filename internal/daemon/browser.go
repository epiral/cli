package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	v1 "github.com/epiral/cli/gen/epiral/v1"
)

const (
	sseHeartbeatInterval = 15 * time.Second
	browserCmdTimeout    = 30 * time.Second
)

// BrowserBridge 管理浏览器插件的 SSE 连接和命令转发。
// 内嵌一个 HTTP 服务，插件通过 SSE 连接接收命令，通过 POST /result 回传结果。
type BrowserBridge struct {
	browserID   string
	description string
	port        int
	daemon      *Daemon

	server   *http.Server
	listener net.Listener

	// SSE 连接管理（单连接模式）
	sseMu      sync.Mutex
	sseWriter  http.ResponseWriter
	sseFlusher http.Flusher
	sseCancel  context.CancelFunc
	connected  bool

	// 请求-响应匹配
	pendingMu sync.Mutex
	pending   map[string]chan string // requestID → result channel
}

// NewBrowserBridge 创建浏览器桥接
func NewBrowserBridge(browserID, description string, port int, d *Daemon) *BrowserBridge {
	if description == "" {
		description = browserID
	}
	return &BrowserBridge{
		browserID:   browserID,
		description: description,
		port:        port,
		daemon:      d,
		pending:     make(map[string]chan string),
	}
}

// Start 启动 HTTP 服务（SSE + /result）
func (b *BrowserBridge) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", b.handleSSE)
	mux.HandleFunc("/result", b.handleResult)
	mux.HandleFunc("/status", b.handleStatus)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", b.port))
	if err != nil {
		return fmt.Errorf("监听端口 %d 失败: %w", b.port, err)
	}
	b.listener = ln
	b.server = &http.Server{
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := b.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[浏览器] HTTP 服务错误: %v", err)
		}
	}()

	return nil
}

// Stop 停止 HTTP 服务
func (b *BrowserBridge) Stop() {
	b.sseMu.Lock()
	if b.sseCancel != nil {
		b.sseCancel()
	}
	b.sseMu.Unlock()

	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = b.server.Shutdown(ctx)
	}

	// reject 所有 pending 请求
	b.pendingMu.Lock()
	for id, ch := range b.pending {
		close(ch)
		delete(b.pending, id)
	}
	b.pendingMu.Unlock()
}

// HandleBrowserExec 处理 Agent 下发的浏览器命令
func (b *BrowserBridge) HandleBrowserExec(requestID string, req *v1.BrowserExecRequest) {
	log.Printf("[浏览器] 收到命令: %s", truncate(req.CommandJson, 80))

	// 检查插件是否连接
	b.sseMu.Lock()
	if !b.connected {
		b.sseMu.Unlock()
		b.daemon.sendBrowserExecOutput(requestID, "", "浏览器插件未连接")
		return
	}
	b.sseMu.Unlock()

	// 注册 pending 请求
	resultCh := make(chan string, 1)
	b.pendingMu.Lock()
	b.pending[requestID] = resultCh
	b.pendingMu.Unlock()

	defer func() {
		b.pendingMu.Lock()
		delete(b.pending, requestID)
		b.pendingMu.Unlock()
	}()

	// 通过 SSE 推送命令给插件
	// command_json 里已有 id 字段（由 Agent 生成），直接转发
	if err := b.sseWrite("command", req.CommandJson); err != nil {
		b.daemon.sendBrowserExecOutput(requestID, "", fmt.Sprintf("SSE 推送失败: %v", err))
		return
	}

	// 等待插件通过 /result 回传结果
	timeout := browserCmdTimeout
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	select {
	case result, ok := <-resultCh:
		if !ok {
			b.daemon.sendBrowserExecOutput(requestID, "", "请求被取消")
			return
		}
		b.daemon.sendBrowserExecOutput(requestID, result, "")
	case <-time.After(timeout):
		b.daemon.sendBrowserExecOutput(requestID, "", fmt.Sprintf("命令执行超时 (%v)", timeout))
	}
}

// handleSSE 处理插件的 SSE 连接
func (b *BrowserBridge) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// 单连接模式：新连接踢掉旧连接
	b.sseMu.Lock()
	if b.sseCancel != nil {
		b.sseCancel()
	}
	ctx, cancel := context.WithCancel(r.Context())
	b.sseWriter = w
	b.sseFlusher = flusher
	b.sseCancel = cancel
	b.connected = true
	b.sseMu.Unlock()

	log.Printf("[浏览器] 插件已连接 (SSE)")
	b.daemon.sendBrowserRegistration(b.browserID, b.description, true)

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	// 发送 connected 事件
	_ = b.sseWriteDirect(w, flusher, "connected", fmt.Sprintf(`{"time":%d}`, time.Now().UnixMilli()))

	// 心跳循环，保持连接
	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto cleanup
		case <-ticker.C:
			if err := b.sseWriteDirect(w, flusher, "heartbeat", fmt.Sprintf(`{"time":%d}`, time.Now().UnixMilli())); err != nil {
				goto cleanup
			}
		}
	}

cleanup:
	b.sseMu.Lock()
	b.connected = false
	b.sseWriter = nil
	b.sseFlusher = nil
	b.sseMu.Unlock()

	log.Printf("[浏览器] 插件已断开")
	b.daemon.sendBrowserRegistration(b.browserID, b.description, false)
}

// handleResult 处理插件回传的命令执行结果
func (b *BrowserBridge) handleResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB 上限
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	// 解析 id 字段用于匹配
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.ID == "" {
		http.Error(w, "invalid result: missing id", http.StatusBadRequest)
		return
	}

	// 匹配 pending 请求
	b.pendingMu.Lock()
	ch, ok := b.pending[result.ID]
	b.pendingMu.Unlock()

	if ok {
		ch <- string(body)
	} else {
		log.Printf("[浏览器] 收到未知 id 的结果: %s", result.ID)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleStatus 返回浏览器桥接状态
func (b *BrowserBridge) handleStatus(w http.ResponseWriter, r *http.Request) {
	b.sseMu.Lock()
	connected := b.connected
	b.sseMu.Unlock()

	b.pendingMu.Lock()
	pendingCount := len(b.pending)
	b.pendingMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"running":            true,
		"extensionConnected": connected,
		"browserID":          b.browserID,
		"pendingRequests":    pendingCount,
	})
}

// sseWrite 通过当前 SSE 连接发送事件
func (b *BrowserBridge) sseWrite(event, data string) error {
	b.sseMu.Lock()
	w := b.sseWriter
	f := b.sseFlusher
	b.sseMu.Unlock()

	if w == nil || f == nil {
		return fmt.Errorf("SSE 未连接")
	}
	return b.sseWriteDirect(w, f, event, data)
}

// sseWriteDirect 直接写入 SSE 事件
func (b *BrowserBridge) sseWriteDirect(w http.ResponseWriter, f http.Flusher, event, data string) error {
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if err != nil {
		return err
	}
	f.Flush()
	return nil
}

// withCORS 添加 CORS 头（Chrome 扩展可能需要）
func withCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

// truncate 截断字符串
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
