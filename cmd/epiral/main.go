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

	"github.com/epiral/cli/internal/config"
	"github.com/epiral/cli/internal/daemon"
	"github.com/epiral/cli/internal/logger"
	"github.com/epiral/cli/internal/webserver"
)

const version = "0.3.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "start" {
		startCmd(os.Args[2:])
		return
	}

	// 传统模式：直连（保留向后兼容）
	legacyCmd()
}

// startCmd 启动 Web 管理面板 + Daemon
func startCmd(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径 (默认 ~/.epiral/config.yaml)")
	webPort := fs.Int("port", 0, "Web 管理面板端口 (默认 19800)")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	// 初始化日志系统：接管 Go 标准 log，写入 ring buffer + stdout
	logBuf := logger.GlobalBuffer()
	log.SetFlags(0) // 清除默认时间前缀，由 LogWriter 统一格式化
	log.SetOutput(logger.NewLogWriter(logBuf))

	log.Printf("[系统] Epiral CLI v%s 启动", version)

	// 加载配置
	cfgPath := *configPath
	if cfgPath == "" {
		p, err := config.DefaultConfigPath()
		if err != nil {
			log.Fatalf("[系统] %v", err)
		}
		cfgPath = p
	}

	store, err := config.NewStore(cfgPath)
	if err != nil {
		log.Fatalf("[系统] 加载配置失败: %v", err)
	}

	cfg := store.Get()
	log.Printf("[系统] 配置文件: %s", cfgPath)

	// Web 端口: 命令行 > 配置文件 > 默认 19800
	port := cfg.Web.Port
	if *webPort > 0 {
		port = *webPort
	}
	if port == 0 {
		port = 19800
	}

	// 上下文和信号处理
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("[系统] 收到退出信号，正在关闭...")
		cancel()
	}()

	// 创建 Daemon Manager
	manager := daemon.NewManager(store)

	// 启动 Web 服务（后台）
	ws := webserver.New(port, store, logBuf, manager, ctx)
	go func() {
		if err := ws.Start(ctx); err != nil {
			log.Printf("[Web] 服务异常: %v", err)
			cancel()
		}
	}()

	// 如果已配置，启动 Daemon
	if cfg.IsConfigured() {
		var modes []string
		if cfg.Computer.ID != "" {
			modes = append(modes, fmt.Sprintf("computer=%s", cfg.Computer.ID))
		}
		if cfg.Browser.ID != "" {
			modes = append(modes, fmt.Sprintf("browser=%s", cfg.Browser.ID))
		}
		log.Printf("[系统] 启动连接: %s → %s", strings.Join(modes, ", "), cfg.Agent.Address)
		manager.Start(ctx)
	} else {
		log.Println("[系统] 未配置连接信息，请在 Web 面板中完成配置")
	}

	// 等待退出
	<-ctx.Done()
	manager.Stop()
	log.Println("[系统] 已关闭")
}

// legacyCmd 传统命令行直连模式
func legacyCmd() {
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
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "用法:")
		fmt.Fprintln(os.Stderr, "  epiral start              启动 Web 管理面板（推荐）")
		fmt.Fprintln(os.Stderr, "  epiral --agent <地址>     直连模式（高级）")
		fmt.Fprintln(os.Stderr, "")
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
	log.Printf("[系统] Epiral CLI 启动 (v%s): %s, agent=%s", version, strings.Join(modes, ", "), cfg.AgentAddr)

	// 自动重连循环
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

		if connDuration > 60*time.Second {
			backoff = time.Second
		}

		log.Printf("[连接] %.0fs 后尝试重连...", backoff.Seconds())
		select {
		case <-ctx.Done():
			break
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	cancel()
}
