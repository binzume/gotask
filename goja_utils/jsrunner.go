package goja_utils

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

const runnerName = "$_js_runner"

func GetJsRunner(runtime *goja.Runtime) *JsRunner {
	rr := runtime.Get(runnerName)
	if rr != nil {
		if r, ok := rr.Export().(*JsRunner); ok {
			return r
		}
	}
	return nil
}

type JsRunner struct {
	*eventloop.EventLoop
	wg       sync.WaitGroup
	vmUnsafe *goja.Runtime // do not use while running
	running  bool
}

func NewJsRunnner() *JsRunner {
	r := &JsRunner{EventLoop: eventloop.NewEventLoop()}
	r.Run(func(vm *goja.Runtime) {
		vm.Set(runnerName, r)
		r.vmUnsafe = vm
	})
	return r
}

func (r *JsRunner) Start() {
	r.running = true
	r.EventLoop.Start()
}

func (r *JsRunner) Stop() (jobs int) {
	jobs = r.EventLoop.Stop()
	r.running = false
	return
}

func (r *JsRunner) IsRunning() bool {
	return r.running
}

func (r *JsRunner) Wait() {
	// TODO: self-implemented eivent loop
	r.Stop()
	for {
		r.Run(func(vm *goja.Runtime) {})
		r.Start()
		r.wg.Wait()
		if r.Stop() == 0 {
			break
		}
	}
}

func (r *JsRunner) Load(path string) (result goja.Value, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r.Run(func(vm *goja.Runtime) {
		result, err = vm.RunScript(filepath.Base(path), string(b))
	})
	return
}

// should be called from JS function
func (r *JsRunner) GoAsync(f func() (any, error)) goja.Value {
	r.wg.Add(1)
	promise, resolve, reject := r.vmUnsafe.NewPromise()
	go func() {
		result, err := f()
		r.RunOnLoop(func(*goja.Runtime) {
			if err == nil {
				resolve(result)
			} else {
				reject(result)
			}
			r.wg.Done()
		})
	}()
	return r.vmUnsafe.ToValue(promise)
}

// should be called from Go function
func (r *JsRunner) Await(value goja.Value) (result goja.Value, ok bool) {
	if p, ok := value.Export().(*goja.Promise); ok {
		ch := make(chan struct{})
		if !r.RunOnLoop(func(vm *goja.Runtime) {
			switch p.State() {
			case goja.PromiseStateRejected:
				result, ok = p.Result(), false
			case goja.PromiseStateFulfilled:
				result, ok = p.Result(), true
			default:
				if f, ok := goja.AssertFunction(value.ToObject(vm).Get("then")); ok {
					f(value, vm.ToValue(func(r goja.Value) { result, ok = r, true }), vm.ToValue(func(r goja.Value) { result, ok = r, false }))
				}
			}
			close(ch)
		}) {
			result, ok = nil, false
			close(ch)
		}
		<-ch
	} else {
		result, ok = value, true
	}
	return
}
