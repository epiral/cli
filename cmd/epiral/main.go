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
	computerID := flag.String("id", "", "电脑 ID (如 my-pc)")
	displayName := flag.String("name", "", "显示名称")
	allowedPaths := flag.String("paths", "", "允许访问的路径，逗号分隔")
	token := flag.String("token", "", "认证 token")
	flag.Parse()

	if *agentAddr == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须指定 --agent 参数")
		flag.Usage()
		os.Exit(1)
	}
	if *computerID == "" {
		hostname, _ := os.Hostname()
		*computerID = hostname
	}
	if *displayName == "" {
		*displayName = *computerID
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
		DisplayName:  *displayName,
		AllowedPaths: paths,
		Token:        *token,
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("收到退出信号，正在关闭...")
		cancel()
	}()

	d := daemon.New(&cfg)
	log.Printf("Epiral CLI 启动: id=%s, agent=%s", cfg.ComputerID, cfg.AgentAddr)

	// 自动重连循环
	for {
		if ctx.Err() != nil {
			break
		}

		err := d.Run(ctx)
		if err == nil {
			// 正常退出（如 ctx 被 cancel）
			break
		}

		if ctx.Err() != nil {
			break
		}

		log.Printf("连接断开: %v", err)
		log.Println("3 秒后尝试重连...")
		select {
		case <-ctx.Done():
			break
		case <-time.After(3 * time.Second):
			// 继续重连
		}
	}
	cancel()
}
