package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func RunSh(ctx context.Context, config *TaskConfig, params map[string]any, log io.Writer) *TaskResult {
	r := &TaskResult{}
	cmd := exec.CommandContext(ctx, "bash", "-c", config.Command)
	cmd.Dir = config.Dir
	cmd.Env = os.Environ()
	if log != nil {
		cmd.Stdout = log
		cmd.Stderr = log
	}
	for n, v := range config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", n, v))
	}
	for n, v := range params {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", n, v))
	}
	_ = cmd.Run()
	code := cmd.ProcessState.ExitCode()

	r.Success = code == 0
	r.Canceled = code != 0 && code == config.CanceledExitCode
	if !r.Success {
		r.Message = "command exited with code " + fmt.Sprint(code)
	}
	return r
}
