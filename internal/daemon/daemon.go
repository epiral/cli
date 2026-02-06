// Package daemon 实现 Epiral CLI 的核心逻辑。
// 作为 Connect RPC client 连接到 Agent 的 HubService，
// 通过双向流接收命令并执行。
package daemon

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/epiral/cli/gen/epiral/v1"
	"github.com/epiral/cli/gen/epiral/v1/epiralv1connect"
	"golang.org/x/net/http2"
)

// Config 是 Daemon 的配置
type Config struct {
	AgentAddr    string   // Agent 地址 (如 http://localhost:50051)
	ComputerID   string   // 电脑 ID（空 = 不注册电脑）
	ComputerDesc string   // 电脑描述
	BrowserID    string   // 浏览器 ID（空 = 不启用浏览器）
	BrowserDesc  string   // 浏览器描述
	BrowserPort  int      // 浏览器 SSE 服务端口（默认 19824）
	AllowedPaths []string // 允许访问的路径
	Token        string   // 认证 token
}

// Daemon 是核心结构
type Daemon struct {
	config   Config
	stream   *connect.BidiStreamForClient[v1.ConnectRequest, v1.ConnectResponse]
	sendMu   sync.Mutex // 保护 stream.Send 的并发安全
	lastPong time.Time
	pongMu   sync.Mutex
	browser  *BrowserBridge // 浏览器桥接（nil = 未启用）
}

// New 创建一个新的 Daemon
func New(cfg *Config) *Daemon {
	return &Daemon{config: *cfg}
}

// Run 启动 Daemon，连接 Agent 并处理命令
func (d *Daemon) Run(ctx context.Context) error {
	log.Printf("[连接] 正在连接 Agent: %s", d.config.AgentAddr)

	// 每次连接都创建新的 HTTP/2 transport，避免复用已损坏的连接
	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, addr)
		},
		ReadIdleTimeout: 30 * time.Second,
		PingTimeout:     10 * time.Second,
	}
	defer transport.CloseIdleConnections()
	h2cClient := &http.Client{Transport: transport}
	client := epiralv1connect.NewHubServiceClient(
		h2cClient,
		d.config.AgentAddr,
	)

	// 建立双向流
	stream := client.Connect(ctx)
	d.stream = stream
	defer func() { _ = stream.CloseRequest() }()

	// 条件注册: Computer
	if d.config.ComputerID != "" {
		reg := d.buildRegistration()
		if err := stream.Send(&v1.ConnectRequest{
			Payload: &v1.ConnectRequest_Registration{Registration: reg},
		}); err != nil {
			return fmt.Errorf("发送 Registration 失败: %w", err)
		}
		log.Printf("[连接] 已注册电脑: %s (%s/%s)", d.config.ComputerID, reg.Os, reg.Arch)
	}

	// 条件注册: Browser — 启动 SSE 服务并等待插件连接
	if d.config.BrowserID != "" {
		d.browser = NewBrowserBridge(d.config.BrowserID, d.config.BrowserDesc, d.config.BrowserPort, d)
		if err := d.browser.Start(ctx); err != nil {
			return fmt.Errorf("启动浏览器 SSE 服务失败: %w", err)
		}
		defer d.browser.Stop()
		log.Printf("[浏览器] SSE 服务已启动: port=%d, id=%s", d.config.BrowserPort, d.config.BrowserID)
	}

	// 初始化 lastPong
	d.pongMu.Lock()
	d.lastPong = time.Now()
	d.pongMu.Unlock()

	// 启动心跳
	heartbeatCtx, heartbeatCancel := context.WithCancelCause(ctx)
	defer heartbeatCancel(nil)
	go d.heartbeat(heartbeatCtx, heartbeatCancel, 3*time.Second)

	log.Println("[连接] 等待 Agent 下发命令...")

	// 主循环：接收命令
	for {
		resp, err := stream.Receive()
		if err != nil {
			if cause := context.Cause(heartbeatCtx); cause != nil && cause != ctx.Err() {
				return cause
			}
			return fmt.Errorf("接收消息失败: %w", err)
		}
		go d.handleMessage(ctx, resp)
	}
}

// handleMessage 分发命令
func (d *Daemon) handleMessage(ctx context.Context, msg *v1.ConnectResponse) {
	switch payload := msg.Payload.(type) {
	case *v1.ConnectResponse_Exec:
		if d.config.ComputerID == "" {
			log.Printf("[连接] 收到 Exec 但未启用电脑功能，忽略")
			return
		}
		d.handleExec(ctx, msg.RequestId, payload.Exec)
	case *v1.ConnectResponse_ReadFile:
		if d.config.ComputerID == "" {
			return
		}
		d.handleReadFile(msg.RequestId, payload.ReadFile)
	case *v1.ConnectResponse_WriteFile:
		if d.config.ComputerID == "" {
			return
		}
		d.handleWriteFile(msg.RequestId, payload.WriteFile)
	case *v1.ConnectResponse_EditFile:
		if d.config.ComputerID == "" {
			return
		}
		d.handleEditFile(msg.RequestId, payload.EditFile)
	case *v1.ConnectResponse_BrowserExec:
		if d.browser == nil {
			log.Printf("[连接] 收到 BrowserExec 但未启用浏览器功能，忽略")
			return
		}
		d.browser.HandleBrowserExec(msg.RequestId, payload.BrowserExec)
	case *v1.ConnectResponse_Pong:
		d.pongMu.Lock()
		d.lastPong = time.Now()
		d.pongMu.Unlock()
	default:
		log.Printf("[连接] 未知消息类型: %T", msg.Payload)
	}
}

// send 发送上行消息（stream.Send 不是并发安全的，用 mutex 串行化）
func (d *Daemon) send(msg *v1.ConnectRequest) error {
	d.sendMu.Lock()
	defer d.sendMu.Unlock()
	return d.stream.Send(msg)
}

// sendBrowserRegistration 发送浏览器注册/状态变更
func (d *Daemon) sendBrowserRegistration(browserID, description string, online bool) {
	if err := d.send(&v1.ConnectRequest{
		Payload: &v1.ConnectRequest_BrowserRegistration{
			BrowserRegistration: &v1.BrowserRegistration{
				BrowserId:   browserID,
				Description: description,
				Online:      online,
			},
		},
	}); err != nil {
		log.Printf("[浏览器] 发送 BrowserRegistration 失败: %v", err)
	}
}

// sendBrowserExecOutput 发送浏览器命令执行结果
func (d *Daemon) sendBrowserExecOutput(requestID, resultJSON, errMsg string) {
	if err := d.send(&v1.ConnectRequest{
		RequestId: requestID,
		Payload: &v1.ConnectRequest_BrowserExecOutput{
			BrowserExecOutput: &v1.BrowserExecOutput{
				ResultJson: resultJSON,
				Error:      errMsg,
				Done:       true,
			},
		},
	}); err != nil {
		log.Printf("[浏览器] 发送 BrowserExecOutput 失败: %v", err)
	}
}

// buildRegistration 构建电脑注册信息
func (d *Daemon) buildRegistration() *v1.Registration {
	homeDir, _ := os.UserHomeDir()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	desc := d.config.ComputerDesc
	if desc == "" {
		desc = d.config.ComputerID
	}

	return &v1.Registration{
		ComputerId:   d.config.ComputerID,
		Description:  desc,
		Os:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Shell:        shell,
		HomeDir:      homeDir,
		Tools:        detectTools(),
		AllowedPaths: d.config.AllowedPaths,
		Token:        d.config.Token,
	}
}

// detectTools 检测本机工具及版本
func detectTools() map[string]string {
	tools := map[string]string{}
	checks := []struct {
		name string
		cmd  string
		args []string
	}{
		{"go", "go", []string{"version"}},
		{"node", "node", []string{"--version"}},
		{"python3", "python3", []string{"--version"}},
		{"git", "git", []string{"--version"}},
		{"docker", "docker", []string{"--version"}},
		{"pnpm", "pnpm", []string{"--version"}},
		{"bun", "bun", []string{"--version"}},
		{"rustc", "rustc", []string{"--version"}},
	}
	for _, c := range checks {
		out, err := exec.Command(c.cmd, c.args...).Output()
		if err == nil {
			ver := strings.TrimSpace(string(out))
			if idx := strings.IndexByte(ver, '\n'); idx > 0 {
				ver = ver[:idx]
			}
			tools[c.name] = ver
		}
	}
	return tools
}

// heartbeat 定期发送心跳，并检测 Pong 超时
func (d *Daemon) heartbeat(ctx context.Context, cancel context.CancelCauseFunc, interval time.Duration) {
	const pongTimeout = 10 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := d.send(&v1.ConnectRequest{
				Payload: &v1.ConnectRequest_Ping{
					Ping: &v1.Ping{Timestamp: time.Now().UnixMilli()},
				},
			})
			if err != nil {
				log.Printf("[心跳] 发送失败: %v", err)
				cancel(fmt.Errorf("心跳发送失败: %w", err))
				return
			}
			d.pongMu.Lock()
			elapsed := time.Since(d.lastPong)
			d.pongMu.Unlock()
			if elapsed > pongTimeout {
				log.Printf("[心跳] Pong 超时 (%.0fs 未收到回应)，主动断连", elapsed.Seconds())
				cancel(fmt.Errorf("Pong 超时 (%.0fs)", elapsed.Seconds()))
				return
			}
		}
	}
}

// isPathAllowed 检查路径权限
func (d *Daemon) isPathAllowed(path string) bool {
	if len(d.config.AllowedPaths) == 0 {
		return true
	}
	for _, allowed := range d.config.AllowedPaths {
		if path == allowed || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}

// shell 返回当前 shell 路径
func (d *Daemon) shell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return shell
}
