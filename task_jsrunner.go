package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/binzume/gotask/goja_utils"
	"github.com/dop251/goja"
)

const InitScript = `
const exports = {};
const module = { exports: exports };
const process = { env: {} };
// returns entry point function.
(async function (cb, data) {
    if (exports.handler) {
        process.env = data.env;
        try {
            let r = await exports.handler(data.event, data.context);
            if (typeof r === 'string') {
                r = { body: r };
            }
            cb(r, true);
        } catch (e) {
            console.log("ERROR", e);
            cb({ error: e })
        }
        return;
    }
    cb({ body: null })
});
`

type JsTaskInstance struct {
	runner  *goja_utils.JsRunner
	f       goja.Callable
	context map[string]any
	Env     map[string]any
}

func StartJsTask(path string) (instance *JsTaskInstance, err error) {
	runner := goja_utils.NewJsRunnner()

	var entryPoint goja.Value
	runner.Run(func(vm *goja.Runtime) {
		goja_utils.EnableFetch(vm)
		entryPoint, err = vm.RunString(InitScript)
	})
	if err != nil {
		return nil, err
	}
	_, err = runner.RunFile(path)
	if err != nil {
		return nil, err
	}
	if f, ok := goja.AssertFunction(entryPoint); ok {
		runner.Start()
		return &JsTaskInstance{runner: runner, f: f, Env: map[string]any{},
			context: map[string]any{
				"name": path,
			}}, nil
	}
	return nil, fmt.Errorf("no entry point")
}

func (l *JsTaskInstance) Execute(params any) (result map[string]any, success bool) {
	data := map[string]any{
		"env":     l.Env,
		"event":   params,
		"context": l.context,
	}
	ch := make(chan struct{})
	l.runner.RunOnLoop(func(vm *goja.Runtime) {
		l.f(goja.Undefined(), vm.ToValue(func(r map[string]any, ok bool) {
			result = r
			success = ok
			close(ch)
		}), vm.ToValue(data))
	})
	<-ch
	return
}

func (l *JsTaskInstance) Wait() {
	l.runner.Wait()
}

func (l *JsTaskInstance) Close() error {
	l.runner.StopNoWait()
	l.runner.Terminate()
	return nil
}

func RunJs(ctx context.Context, config *TaskConfig, params map[string]any, log io.Writer) *TaskResult {
	r := &TaskResult{}
	s, err := StartJsTask(filepath.Join(config.Dir, config.Command))
	if err != nil {
		r.Message = "Failed to load:" + config.Command
	} else {
		for n, v := range config.Env {
			s.Env[n] = v
		}
		ret, ok := s.Execute(params)
		if log != nil {
			fmt.Fprintln(log, ret)
		}
		r.Result = ret
		r.Success = ok
		if !ok {
			r.Message = fmt.Sprint(ret)
		}
		s.Close()
	}

	return r
}
