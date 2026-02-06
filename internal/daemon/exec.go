package daemon

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	v1 "github.com/epiral/cli/gen/epiral/v1"
)

const defaultTimeoutMs = 30000

// handleExec 执行命令，流式返回输出
func (d *Daemon) handleExec(ctx context.Context, requestID string, req *v1.ExecRequest) {
	// 命令摘要（截断过长的命令）
	cmdSummary := req.Command
	if len(cmdSummary) > 80 {
		cmdSummary = cmdSummary[:77] + "..."
	}
	log.Printf("[执行] $ %s", cmdSummary)
	execStart := time.Now()

	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// 工作目录
	workdir := req.Workdir
	if workdir == "" {
		workdir, _ = os.UserHomeDir()
	}
	if !d.isPathAllowed(workdir) {
		log.Printf("[执行] 拒绝: 路径不允许 %s", workdir)
		d.sendExecDone(requestID, "", fmt.Sprintf("路径不允许: %s", workdir), 1, workdir)
		return
	}

	// 创建命令
	cmd := exec.CommandContext(execCtx, d.shell(), "-c", req.Command)
	cmd.Dir = workdir
	cmd.Env = os.Environ()

	// stdout 管道（流式）
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		d.sendExecDone(requestID, "", fmt.Sprintf("stdout 管道失败: %v", err), 1, workdir)
		return
	}
	// stderr 管道
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		d.sendExecDone(requestID, "", fmt.Sprintf("stderr 管道失败: %v", err), 1, workdir)
		return
	}

	if err := cmd.Start(); err != nil {
		d.sendExecDone(requestID, "", fmt.Sprintf("启动失败: %v", err), 1, workdir)
		return
	}

	// 流式发送 stdout
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			if err := d.send(&v1.ConnectRequest{
				RequestId: requestID,
				Payload: &v1.ConnectRequest_ExecOutput{
					ExecOutput: &v1.ExecOutput{
						Stdout: scanner.Text() + "\n",
					},
				},
			}); err != nil {
				log.Printf("[执行] 发送 stdout 失败: %v", err)
				return
			}
		}
	}()

	// 收集 stderr
	stderrBytes, _ := io.ReadAll(io.LimitReader(stderrPipe, 100*1024))

	// 等待完成
	var exitCode int32
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = int32(exitErr.ExitCode()) //nolint:gosec // exit code 不会溢出 int32
		} else if execCtx.Err() == context.DeadlineExceeded {
			exitCode = 124
		} else {
			exitCode = 1
		}
	}

	elapsed := time.Since(execStart)
	if exitCode == 0 {
		log.Printf("[执行] 完成 (%.1fs)", elapsed.Seconds())
	} else {
		log.Printf("[执行] 失败 exit=%d (%.1fs)", exitCode, elapsed.Seconds())
	}

	d.sendExecDone(requestID, "", string(stderrBytes), exitCode, workdir)
}

// sendExecDone 发送执行完成消息
func (d *Daemon) sendExecDone(requestID, stdout, stderr string, exitCode int32, workdir string) {
	if err := d.send(&v1.ConnectRequest{
		RequestId: requestID,
		Payload: &v1.ConnectRequest_ExecOutput{
			ExecOutput: &v1.ExecOutput{
				Stdout:   stdout,
				Stderr:   stderr,
				ExitCode: exitCode,
				Done:     true,
				Workdir:  workdir,
			},
		},
	}); err != nil {
		log.Printf("[执行] 发送结果失败: %v", err)
	}
}
