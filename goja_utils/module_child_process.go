package goja_utils

import (
	"bytes"
	"os/exec"
	"runtime"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

func runInSell(cmd string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/s", "/c", cmd}
	} else {
		return "/bin/sh", []string{"-c", cmd}
	}
}

func convOutput(data []byte, vm *goja.Runtime, options map[string]any) any {
	if options != nil {
		switch options["encoding"] {
		case "bytes":
			return data
		case "buffer":
			return vm.NewArrayBuffer(data)
		}
	}

	return string(data)
}

func makeExec(vm *goja.Runtime) any {
	return func(cmd string, callback func(error, any, any), options map[string]any) any {
		cmd, args := runInSell(cmd)
		return makeExecFile(vm)(cmd, args, callback, options)
	}
}

func makeExecFile(vm *goja.Runtime) func(cmd string, args []string, callback func(error, any, any), options map[string]any) any {
	r := GetTaskQueue(vm)
	return func(cmd string, args []string, callback func(error, any, any), options map[string]any) any {
		c := exec.Command(cmd, args...)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		err := c.Start()
		if err != nil {
			return err
		}
		p := r.StartGoroutineTask(func() (any, error) {
			c.Wait()
			r.QueueMicrotask(func(r *goja.Runtime) {
				callback(err, convOutput(stdout.Bytes(), vm, options), convOutput(stderr.Bytes(), vm, options))
			})
			return nil, err
		})
		o := p.ToObject(vm)
		o.Set("pid", c.Process.Pid)
		o.Set("kill", c.Process.Kill)
		o.DefineAccessorProperty("exitCode", vm.ToValue(func() int {
			return c.ProcessState.ExitCode()
		}), nil, goja.FLAG_FALSE, goja.FLAG_FALSE)

		return p
	}
}

func makeExecSync(vm *goja.Runtime) any {
	return func(cmd string, options map[string]any) any {
		cmd, args := runInSell(cmd)
		return makeExecFileSync(vm)(cmd, args, options)
	}
}

func makeExecFileSync(vm *goja.Runtime) func(cmd string, args []string, options map[string]any) any {
	return func(cmd string, args []string, options map[string]any) any {
		c, err := exec.Command(cmd, args...).Output()
		if err != nil {
			return nil
		}
		return convOutput(c, vm, options)
	}

}

func spawnSync(cmd string, args []string, options map[string]any) any {
	c := exec.Command(cmd, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		return nil
	}
	return map[string]any{"output": string(stdout.String()), "status": c.ProcessState.ExitCode()}
}

func RequireChildProcess(runtime *goja.Runtime, module *goja.Object) {
	o := module.Get("exports").(*goja.Object)
	o.Set("exec", makeExec(runtime))
	o.Set("execFile", makeExecFile(runtime))
	o.Set("execSync", makeExecSync(runtime))
	o.Set("execFileSync", makeExecFileSync(runtime))
	o.Set("spawnSync", spawnSync)
}

func EnablChildProcess(runtime *goja.Runtime) {
	runtime.Set("child_process", require.Require(runtime, "child_process"))
}

func init() {
	require.RegisterCoreModule("child_process", RequireChildProcess)
}
