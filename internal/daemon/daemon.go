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
	config Config
	stream *connect.BidiStreamForClient[v1.ConnectRequest, v1.ConnectResponse]
}

// New 创建一个新的 Daemon
func New(cfg *Config) *Daemon {
	return &Daemon{config: *cfg}
}

// Run 启动 Daemon，连接 Agent 并处理命令
func (d *Daemon) Run(ctx context.Context) error {
	log.Printf("连接 Agent: %s", d.config.AgentAddr)

	// 创建 HTTP/2 client（h2c，支持 bidi streaming）
	h2cClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
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

	// 启动心跳
	go d.heartbeat(ctx, 30*time.Second)

	// 主循环：接收命令
	for {
		resp, err := stream.Receive()
		if err != nil {
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
		// 心跳回应
	default:
		log.Printf("未知消息类型: %T", msg.Payload)
	}
}

// send 发送上行消息（stream.Send 本身不是并发安全的，需要串行化）
// TODO: P1 加 channel 串行化
func (d *Daemon) send(msg *v1.ConnectRequest) error {
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

// heartbeat 定期发送心跳
func (d *Daemon) heartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = d.send(&v1.ConnectRequest{
				Payload: &v1.ConnectRequest_Ping{
					Ping: &v1.Ping{Timestamp: time.Now().UnixMilli()},
				},
			})
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
