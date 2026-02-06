// Package daemon 实现 Epiral CLI 的核心逻辑。
// 作为 Connect RPC client 连接到 Agent 的 ComputerHubService，
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
	ComputerID   string   // 电脑 ID (如 "my-pc")
	DisplayName  string   // 显示名称
	AllowedPaths []string // 允许访问的路径
	Token        string   // 认证 token
}

// Daemon 是电脑端的核心结构
type Daemon struct {
	config   Config
	stream   *connect.BidiStreamForClient[v1.ConnectRequest, v1.ConnectResponse]
	sendMu   sync.Mutex // 保护 stream.Send 的并发安全
	lastPong time.Time  // 最近一次收到 Pong 的时间
	pongMu   sync.Mutex
}

// New 创建一个新的 Daemon
func New(cfg *Config) *Daemon {
	return &Daemon{config: *cfg}
}

// Run 启动 Daemon，连接 Agent 并处理命令
func (d *Daemon) Run(ctx context.Context) error {
	log.Printf("连接 Agent: %s", d.config.AgentAddr)

	// 每次连接都创建新的 HTTP/2 transport，避免复用已损坏的连接
	// ReadIdleTimeout: 30s 无数据时发送 HTTP/2 PING 帧
	// PingTimeout: 10s 内没有 PING ACK 则关闭连接
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
	client := epiralv1connect.NewComputerHubServiceClient(
		h2cClient,
		d.config.AgentAddr,
	)

	// 建立双向流
	stream := client.Connect(ctx)
	d.stream = stream
	defer func() { _ = stream.CloseRequest() }()

	// 发送 Registration
	reg := d.buildRegistration()
	if err := stream.Send(&v1.ConnectRequest{
		Payload: &v1.ConnectRequest_Registration{Registration: reg},
	}); err != nil {
		return fmt.Errorf("发送 Registration 失败: %w", err)
	}
	log.Printf("已注册: %s (%s/%s)", d.config.ComputerID, reg.Os, reg.Arch)

	// 初始化 lastPong
	d.pongMu.Lock()
	d.lastPong = time.Now()
	d.pongMu.Unlock()

	// 启动心跳（3 秒间隔），带 Pong 超时检测（10s）
	heartbeatCtx, heartbeatCancel := context.WithCancelCause(ctx)
	defer heartbeatCancel(nil)
	go d.heartbeat(heartbeatCtx, heartbeatCancel, 3*time.Second)

	// 主循环：接收命令
	for {
		resp, err := stream.Receive()
		if err != nil {
			// 区分心跳超时和其他错误
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
		d.handleExec(ctx, msg.RequestId, payload.Exec)
	case *v1.ConnectResponse_ReadFile:
		d.handleReadFile(msg.RequestId, payload.ReadFile)
	case *v1.ConnectResponse_WriteFile:
		d.handleWriteFile(msg.RequestId, payload.WriteFile)
	case *v1.ConnectResponse_EditFile:
		d.handleEditFile(msg.RequestId, payload.EditFile)
	case *v1.ConnectResponse_Pong:
		d.pongMu.Lock()
		d.lastPong = time.Now()
		d.pongMu.Unlock()
	default:
		log.Printf("未知消息类型: %T", msg.Payload)
	}
}

// send 发送上行消息（stream.Send 不是并发安全的，用 mutex 串行化）
func (d *Daemon) send(msg *v1.ConnectRequest) error {
	d.sendMu.Lock()
	defer d.sendMu.Unlock()
	return d.stream.Send(msg)
}

// buildRegistration 构建注册信息
func (d *Daemon) buildRegistration() *v1.Registration {
	homeDir, _ := os.UserHomeDir()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	return &v1.Registration{
		ComputerId:   d.config.ComputerID,
		DisplayName:  d.config.DisplayName,
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
// 如果连续 pongTimeout（10s）没收到 Pong，主动取消连接触发重连
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
				log.Printf("心跳发送失败: %v", err)
				cancel(fmt.Errorf("心跳发送失败: %w", err))
				return
			}
			// 检查 Pong 超时
			d.pongMu.Lock()
			elapsed := time.Since(d.lastPong)
			d.pongMu.Unlock()
			if elapsed > pongTimeout {
				log.Printf("Pong 超时 (%.0fs 未收到回应)，主动断连", elapsed.Seconds())
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
