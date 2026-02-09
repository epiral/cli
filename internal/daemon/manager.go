package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/epiral/cli/internal/config"
)

// ConnectionState 连接状态
type ConnectionState string

const (
	StateStopped      ConnectionState = "stopped"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateReconnecting ConnectionState = "reconnecting"
	StateError        ConnectionState = "error"
)

// Status 对外暴露的状态快照
type Status struct {
	State       ConnectionState `json:"state"`
	ConnectedAt *time.Time      `json:"connectedAt,omitempty"`
	Uptime      string          `json:"uptime,omitempty"`
	Reconnects  int             `json:"reconnects"`
	LastError   string          `json:"lastError,omitempty"`
	Computer    string          `json:"computer,omitempty"`
}

// Manager 管理 Daemon 的生命周期（启动、重连、停止、重启）
type Manager struct {
	mu          sync.RWMutex
	state       ConnectionState
	lastError   string
	connectedAt time.Time
	reconnects  int

	configStore *config.Store

	cancel    context.CancelFunc
	done      chan struct{}
	restartMu sync.Mutex // 防止并发 Restart
}

// NewManager 创建 Daemon 管理器
func NewManager(store *config.Store) *Manager {
	return &Manager{
		configStore: store,
		state:       StateStopped,
	}
}

// Start 启动 Daemon（在后台 goroutine 运行重连循环）
func (m *Manager) Start(ctx context.Context) {
	m.restartMu.Lock()
	defer m.restartMu.Unlock()

	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return // 已经在运行
	}

	daemonCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.done = make(chan struct{})
	m.reconnects = 0
	m.lastError = ""
	m.mu.Unlock()

	go m.run(daemonCtx)
}

// Stop 停止 Daemon
func (m *Manager) Stop() {
	m.restartMu.Lock()
	defer m.restartMu.Unlock()
	m.stop()
}

// Restart 重启 Daemon（用新配置）
func (m *Manager) Restart(ctx context.Context) {
	m.restartMu.Lock()
	defer m.restartMu.Unlock()

	// 先停
	m.stop()
	// 再启
	m.mu.Lock()
	daemonCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.done = make(chan struct{})
	m.reconnects = 0
	m.lastError = ""
	m.mu.Unlock()

	go m.run(daemonCtx)
}

// stop 内部停止（不加 restartMu）
func (m *Manager) stop() {
	m.mu.Lock()
	cancel := m.cancel
	done := m.done
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	m.mu.Lock()
	m.cancel = nil
	m.done = nil
	m.state = StateStopped
	m.mu.Unlock()
}

// Status 返回当前状态快照
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.configStore.Get()
	s := Status{
		State:      m.state,
		Reconnects: m.reconnects,
		LastError:  m.lastError,
		Computer:   cfg.Computer.ID,
	}

	if m.state == StateConnected && !m.connectedAt.IsZero() {
		t := m.connectedAt
		s.ConnectedAt = &t
		duration := time.Since(m.connectedAt)
		s.Uptime = formatDuration(duration)
	}

	return s
}

// run 重连循环（从 main.go 移入）
func (m *Manager) run(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		close(m.done)
		m.mu.Unlock()
	}()

	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		cfg := m.configStore.Get()
		if !cfg.IsConfigured() {
			m.setState(StateStopped)
			log.Println("[管理] 未配置 Agent 地址或 ID，等待配置...")
			// 每 3 秒检查一次配置
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}

		daemonCfg := buildDaemonConfig(&cfg)
		d := New(&daemonCfg)

		// 设置连接成功回调
		d.OnConnected = func() {
			m.mu.Lock()
			m.state = StateConnected
			m.connectedAt = time.Now()
			m.mu.Unlock()
		}

		m.setState(StateConnecting)

		connectStart := time.Now()
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
					log.Printf("[连接] panic 已恢复: %v", r)
				}
			}()
			err = d.Run(ctx)
		}()
		if err == nil || ctx.Err() != nil {
			return
		}

		connDuration := time.Since(connectStart)

		m.mu.Lock()
		m.state = StateReconnecting
		m.lastError = err.Error()
		m.reconnects++
		m.mu.Unlock()

		log.Printf("[连接] 断开: %v (持续 %.0fs)", err, connDuration.Seconds())

		// 连接维持超过 60s 则重置退避
		if connDuration > 60*time.Second {
			backoff = time.Second
		}

		log.Printf("[连接] %.0fs 后尝试重连...", backoff.Seconds())
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (m *Manager) setState(state ConnectionState) {
	m.mu.Lock()
	m.state = state
	m.mu.Unlock()
}

// buildDaemonConfig 将 config.Config 转为 daemon.Config
func buildDaemonConfig(cfg *config.Config) Config {
	return Config{
		AgentAddr:    cfg.Agent.Address,
		ComputerID:   cfg.Computer.ID,
		ComputerDesc: cfg.Computer.Description,
		AllowedPaths: cfg.Computer.AllowedPaths,
		Token:        cfg.Agent.Token,
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
