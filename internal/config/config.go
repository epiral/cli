// Package config 管理 CLI 的持久化配置。
// 配置文件存储在 ~/.epiral/config.yaml。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config 是 CLI 的完整配置
type Config struct {
	Agent    AgentConfig    `yaml:"agent" json:"agent"`
	Computer ComputerConfig `yaml:"computer" json:"computer"`
	Browser  BrowserConfig  `yaml:"browser" json:"browser"`
	Web      WebConfig      `yaml:"web" json:"web"`
}

// AgentConfig Agent 连接配置
type AgentConfig struct {
	Address string `yaml:"address" json:"address"`
	Token   string `yaml:"token" json:"token"`
}

// ComputerConfig 电脑配置
type ComputerConfig struct {
	ID           string   `yaml:"id" json:"id"`
	Description  string   `yaml:"description" json:"description"`
	AllowedPaths []string `yaml:"allowed_paths" json:"allowedPaths"`
}

// BrowserConfig 浏览器桥接配置
type BrowserConfig struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
	Port        int    `yaml:"port" json:"port"`
}

// WebConfig Web 管理面板配置
type WebConfig struct {
	Port int `yaml:"port" json:"port"`
}

// IsConfigured 返回是否已配置最低限度的连接信息
func (c *Config) IsConfigured() bool {
	return c.Agent.Address != "" && (c.Computer.ID != "" || c.Browser.ID != "")
}

// DefaultConfigDir 返回 ~/.epiral
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取 home 目录失败: %w", err)
	}
	return filepath.Join(home, ".epiral"), nil
}

// DefaultConfigPath 返回 ~/.epiral/config.yaml
func DefaultConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Default 返回带默认值的配置
func Default() *Config {
	return &Config{
		Browser: BrowserConfig{Port: 19824},
		Web:     WebConfig{Port: 19800},
	}
}

// Load 从文件读取配置。文件不存在时返回默认配置。
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 确保默认值
	if cfg.Browser.Port == 0 {
		cfg.Browser.Port = 19824
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = 19800
	}

	return cfg, nil
}

// Save 将配置写入文件
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// Store 封装 Config，提供线程安全的读写和文件持久化
type Store struct {
	mu   sync.RWMutex
	cfg  *Config
	path string
}

// NewStore 创建配置存储，从文件加载初始配置
func NewStore(path string) (*Store, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Store{cfg: cfg, path: path}, nil
}

// Get 返回当前配置的拷贝
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := *s.cfg
	// 深拷贝 slice
	if s.cfg.Computer.AllowedPaths != nil {
		c.Computer.AllowedPaths = make([]string, len(s.cfg.Computer.AllowedPaths))
		copy(c.Computer.AllowedPaths, s.cfg.Computer.AllowedPaths)
	}
	return c
}

// Update 更新配置并持久化到文件
func (s *Store) Update(cfg *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := Save(s.path, cfg); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

// Path 返回配置文件路径
func (s *Store) Path() string {
	return s.path
}
