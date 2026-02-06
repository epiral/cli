package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/epiral/cli/internal/daemon"
)

func main() {
	agentAddr := flag.String("agent", "", "Agent 地址 (如 http://localhost:50051)")
	computerID := flag.String("computer-id", "", "电脑 ID (如 my-pc)")
	computerDesc := flag.String("computer-desc", "", "电脑描述")
	browserID := flag.String("browser-id", "", "浏览器 ID (如 my-chrome)")
	browserDesc := flag.String("browser-desc", "", "浏览器描述")
	browserPort := flag.Int("browser-port", 19824, "浏览器 SSE 服务端口")
	allowedPaths := flag.String("paths", "", "允许访问的路径，逗号分隔")
	token := flag.String("token", "", "认证 token")
	flag.Parse()

	if *agentAddr == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须指定 --agent 参数")
		flag.Usage()
		os.Exit(1)
	}
	if *computerID == "" && *browserID == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须指定 --computer-id 或 --browser-id（至少一个）")
		flag.Usage()
		os.Exit(1)
	}

	var paths []string
	if *allowedPaths != "" {
		for _, p := range strings.Split(*allowedPaths, ",") {
			paths = append(paths, strings.TrimSpace(p))
		}
	}

	cfg := daemon.Config{
		AgentAddr:    *agentAddr,
		ComputerID:   *computerID,
		ComputerDesc: *computerDesc,
		BrowserID:    *browserID,
		BrowserDesc:  *browserDesc,
		BrowserPort:  *browserPort,
		AllowedPaths: paths,
		Token:        *token,
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("[系统] 收到退出信号，正在关闭...")
		cancel()
	}()

	d := daemon.New(&cfg)

	// 启动日志
	var modes []string
	if cfg.ComputerID != "" {
		modes = append(modes, fmt.Sprintf("computer=%s", cfg.ComputerID))
	}
	if cfg.BrowserID != "" {
		modes = append(modes, fmt.Sprintf("browser=%s (port %d)", cfg.BrowserID, cfg.BrowserPort))
	}
	log.Printf("[系统] Epiral CLI 启动 (v0.2.0): %s, agent=%s", strings.Join(modes, ", "), cfg.AgentAddr)

	// 自动重连循环（指数退避：1s → 2s → 4s → ... → 30s 上限，连接成功后重置）
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			break
		}

		connectStart := time.Now()
		err := d.Run(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			break
		}

		connDuration := time.Since(connectStart)
		log.Printf("[连接] 断开: %v (持续 %.0fs)", err, connDuration.Seconds())

		// 如果连接维持了超过 60s，说明之前是正常的，重置退避
		if connDuration > 60*time.Second {
			backoff = time.Second
		}

		log.Printf("[连接] %.0fs 后尝试重连...", backoff.Seconds())
		select {
		case <-ctx.Done():
			break
		case <-time.After(backoff):
		}

		// 指数退避
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	cancel()
}
