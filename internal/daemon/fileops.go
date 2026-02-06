package daemon

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/epiral/cli/gen/epiral/v1"
)

const (
	defaultMaxFileSize = 256 * 1024
	defaultLineLimit   = 2000
)

// handleReadFile 读取文件
func (d *Daemon) handleReadFile(requestID string, req *v1.ReadFileRequest) {
	path := req.Path
	if !d.isPathAllowed(path) {
		d.sendFileContent(requestID, "", 0, 0, fmt.Sprintf("路径不允许: %s", path))
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		d.sendFileContent(requestID, "", 0, 0, fmt.Sprintf("文件不存在: %s", path))
		return
	}
	if info.IsDir() {
		d.sendFileContent(requestID, "", 0, 0, fmt.Sprintf("路径是目录: %s", path))
		return
	}

	maxSize := req.MaxSize
	if maxSize <= 0 {
		maxSize = defaultMaxFileSize
	}
	if info.Size() > maxSize {
		d.sendFileContent(requestID, "", 0, info.Size(),
			fmt.Sprintf("文件过大: %d 字节（上限 %d）", info.Size(), maxSize))
		return
	}

	file, err := os.Open(path)
	if err != nil {
		d.sendFileContent(requestID, "", 0, 0, fmt.Sprintf("打开失败: %v", err))
		return
	}
	defer file.Close()

	offset := int(req.Offset)
	limit := int(req.Limit)
	if limit <= 0 {
		limit = defaultLineLimit
	}

	var lines []string
	totalLines := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		totalLines++
		if totalLines-1 < offset {
			continue
		}
		if len(lines) < limit {
			lines = append(lines, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		d.sendFileContent(requestID, "", 0, info.Size(), fmt.Sprintf("读取失败: %v", err))
		return
	}

	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	d.sendFileContent(requestID, content, int64(totalLines), info.Size(), "")
}

// handleWriteFile 写入文件
func (d *Daemon) handleWriteFile(requestID string, req *v1.WriteFileRequest) {
	if !d.isPathAllowed(req.Path) {
		d.sendOpResult(requestID, false, fmt.Sprintf("路径不允许: %s", req.Path))
		return
	}
	if err := os.MkdirAll(filepath.Dir(req.Path), 0o755); err != nil {
		d.sendOpResult(requestID, false, fmt.Sprintf("创建目录失败: %v", err))
		return
	}
	if err := os.WriteFile(req.Path, []byte(req.Content), 0o600); err != nil {
		d.sendOpResult(requestID, false, fmt.Sprintf("写入失败: %v", err))
		return
	}
	d.sendOpResult(requestID, true, "")
}

// handleEditFile 编辑文件（查找替换）
func (d *Daemon) handleEditFile(requestID string, req *v1.EditFileRequest) {
	if !d.isPathAllowed(req.Path) {
		d.sendOpResult(requestID, false, fmt.Sprintf("路径不允许: %s", req.Path))
		return
	}

	data, err := os.ReadFile(req.Path)
	if err != nil {
		d.sendOpResult(requestID, false, fmt.Sprintf("读取失败: %v", err))
		return
	}
	content := string(data)

	if req.OldString == "" {
		d.sendOpResult(requestID, false, "old_string 不能为空")
		return
	}

	count := strings.Count(content, req.OldString)
	if count == 0 {
		d.sendOpResult(requestID, false, "old_string 未找到")
		return
	}
	if !req.ReplaceAll && count > 1 {
		d.sendOpResult(requestID, false,
			fmt.Sprintf("old_string 出现 %d 次，需更多上下文或使用 replace_all", count))
		return
	}

	var newContent string
	if req.ReplaceAll {
		newContent = strings.ReplaceAll(content, req.OldString, req.NewString)
	} else {
		newContent = strings.Replace(content, req.OldString, req.NewString, 1)
	}

	if err := os.WriteFile(req.Path, []byte(newContent), 0o600); err != nil {
		d.sendOpResult(requestID, false, fmt.Sprintf("写回失败: %v", err))
		return
	}
	d.sendOpResult(requestID, true, "")
}

// sendFileContent 发送文件内容
func (d *Daemon) sendFileContent(requestID, content string, totalLines, fileSize int64, errMsg string) {
	if err := d.send(&v1.ConnectRequest{
		RequestId: requestID,
		Payload: &v1.ConnectRequest_FileContent{
			FileContent: &v1.FileContent{
				Content:    content,
				TotalLines: totalLines,
				FileSize:   fileSize,
				Error:      errMsg,
			},
		},
	}); err != nil {
		log.Printf("发送 FileContent 失败: %v", err)
	}
}

// sendOpResult 发送操作结果
func (d *Daemon) sendOpResult(requestID string, success bool, errMsg string) {
	if err := d.send(&v1.ConnectRequest{
		RequestId: requestID,
		Payload: &v1.ConnectRequest_OpResult{
			OpResult: &v1.OpResult{
				Success: success,
				Error:   errMsg,
			},
		},
	}); err != nil {
		log.Printf("发送 OpResult 失败: %v", err)
	}
}
