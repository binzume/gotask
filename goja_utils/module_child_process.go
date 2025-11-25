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

func Exec(r *JsRunner) any {
	return func(cmd string, callback func(error, any, any), options map[string]any) any {
		cmd, args := runInSell(cmd)
		return ExecFile(r)(cmd, args, callback, options)
	}
}

func ExecFile(r *JsRunner) func(cmd string, args []string, callback func(error, any, any), options map[string]any) any {
	return func(cmd string, args []string, callback func(error, any, any), options map[string]any) any {
		vm := r.vmUnsafe
		c := exec.Command(cmd, args...)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		err := c.Start()
		if err != nil {
			return err
		}
		p := r.GoAsync(func() (any, error) {
			c.Wait()
			r.RunOnLoop(func(r *goja.Runtime) {
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

func ExecSync(vm *goja.Runtime) any {
	return func(cmd string, options map[string]any) any {
		cmd, args := runInSell(cmd)
		return ExecFileSync(vm)(cmd, args, options)
	}
}

func ExecFileSync(vm *goja.Runtime) func(cmd string, args []string, options map[string]any) any {
	return func(cmd string, args []string, options map[string]any) any {
		c, err := exec.Command(cmd, args...).Output()
		if err != nil {
			return nil
		}
		return convOutput(c, vm, options)
	}

}

func SpawnSync(cmd string, args []string, options map[string]any) any {
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
	o.Set("exec", Exec(GetJsRunner(runtime)))
	o.Set("execFile", ExecFile(GetJsRunner(runtime)))
	o.Set("execSync", ExecSync(runtime))
	o.Set("execFileSync", ExecFileSync(runtime))
	o.Set("spawnSync", SpawnSync)
}

func EnablChildProcess(runtime *goja.Runtime) {
	runtime.Set("child_process", require.Require(runtime, "child_process"))
}

func init() {
	require.RegisterCoreModule("child_process", RequireChildProcess)
}
