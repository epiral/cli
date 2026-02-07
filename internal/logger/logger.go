// Package logger 提供带 ring buffer 的日志系统。
// 日志同时输出到 stdout 和内存缓冲区，Web UI 通过 SSE 实时读取。
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level 日志级别
type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Entry 日志条目
type Entry struct {
	Time    time.Time `json:"time"`
	Level   Level     `json:"level"`
	Module  string    `json:"module"`
	Message string    `json:"message"`
}

// JSON 序列化日志条目
func (e *Entry) JSON() []byte {
	data, _ := json.Marshal(e)
	return data
}

// RingBuffer 环形缓冲区，存储最近 N 条日志
type RingBuffer struct {
	mu      sync.RWMutex
	entries []Entry
	size    int
	pos     int
	full    bool

	// SSE 订阅者
	subMu sync.RWMutex
	subs  map[chan Entry]struct{}
}

// NewRingBuffer 创建指定大小的环形缓冲区
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		entries: make([]Entry, size),
		size:    size,
		subs:    make(map[chan Entry]struct{}),
	}
}

// Add 添加一条日志到缓冲区，并通知所有订阅者
func (rb *RingBuffer) Add(entry Entry) {
	rb.mu.Lock()
	rb.entries[rb.pos] = entry
	rb.pos = (rb.pos + 1) % rb.size
	if rb.pos == 0 {
		rb.full = true
	}
	rb.mu.Unlock()

	// 通知所有 SSE 订阅者
	rb.subMu.RLock()
	for ch := range rb.subs {
		select {
		case ch <- entry:
		default:
			// channel 满了就丢弃，避免阻塞
		}
	}
	rb.subMu.RUnlock()
}

// All 返回缓冲区内所有日志（按时间排序）
func (rb *RingBuffer) All() []Entry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if !rb.full {
		result := make([]Entry, rb.pos)
		copy(result, rb.entries[:rb.pos])
		return result
	}

	// 从最旧的位置开始，绕一圈
	result := make([]Entry, rb.size)
	copy(result, rb.entries[rb.pos:])
	copy(result[rb.size-rb.pos:], rb.entries[:rb.pos])
	return result
}

// Subscribe 创建一个 SSE 订阅通道
func (rb *RingBuffer) Subscribe() chan Entry {
	ch := make(chan Entry, 100)
	rb.subMu.Lock()
	rb.subs[ch] = struct{}{}
	rb.subMu.Unlock()
	return ch
}

// Unsubscribe 取消订阅
func (rb *RingBuffer) Unsubscribe(ch chan Entry) {
	rb.subMu.Lock()
	delete(rb.subs, ch)
	rb.subMu.Unlock()
	close(ch)
}

// 全局 buffer
var (
	globalBuf *RingBuffer
	bufOnce   sync.Once
)

// GlobalBuffer 返回全局日志缓冲区（1000 条）
func GlobalBuffer() *RingBuffer {
	bufOnce.Do(func() {
		globalBuf = NewRingBuffer(1000)
	})
	return globalBuf
}

// LogWriter 实现 io.Writer，用于接管 Go 标准 log 包的输出。
// 将日志解析后写入 RingBuffer 和 stdout。
type LogWriter struct {
	buf    *RingBuffer
	stdout io.Writer
}

// NewLogWriter 创建日志写入器
func NewLogWriter(buf *RingBuffer) *LogWriter {
	return &LogWriter{buf: buf, stdout: os.Stdout}
}

// Write 实现 io.Writer。解析 [模块] 前缀，推断日志级别。
func (w *LogWriter) Write(p []byte) (n int, err error) {
	text := strings.TrimRight(string(p), "\n")
	if text == "" {
		return len(p), nil
	}

	// 解析 [模块] 前缀
	module := "系统"
	message := text
	if strings.HasPrefix(text, "[") {
		if idx := strings.Index(text, "] "); idx > 0 {
			module = text[1:idx]
			message = text[idx+2:]
		}
	}

	// 推断日志级别
	level := LevelInfo
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "失败") || strings.Contains(lower, "错误") || strings.Contains(lower, "error"):
		level = LevelError
	case strings.Contains(lower, "警告") || strings.Contains(lower, "warn"):
		level = LevelWarn
	case strings.Contains(lower, "调试") || strings.Contains(lower, "debug"):
		level = LevelDebug
	}

	entry := Entry{
		Time:    time.Now(),
		Level:   level,
		Module:  module,
		Message: message,
	}
	w.buf.Add(entry)

	// 同时输出到 stdout
	timeStr := entry.Time.Format("15:04:05")
	fmt.Fprintf(w.stdout, "%s [%s] [%s] %s\n", timeStr, entry.Level, module, message)

	return len(p), nil
}
