package goja_utils

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

const taskQueueName = "$_task_queue"

type TaskQueue interface {
	// RunOnLoop adds microtask
	QueueMicrotask(func(vm *goja.Runtime)) bool
	// StartGoroutineTask returns Promise
	StartGoroutineTask(f func() (any, error)) goja.Value
	// Wait for finish all tasks
	Wait()
}

func GetTaskQueue(runtime *goja.Runtime) TaskQueue {
	rr := runtime.Get(taskQueueName)
	if rr != nil {
		if r, ok := rr.Export().(TaskQueue); ok {
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
		vm.Set(taskQueueName, r)
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

func (r *JsRunner) StopNoWait() {
	r.EventLoop.StopNoWait()
	r.running = false
}

func (r *JsRunner) IsRunning() bool {
	return r.running
}

func (r *JsRunner) QueueMicrotask(f func(vm *goja.Runtime)) bool {
	return r.RunOnLoop(f)
}

func (r *JsRunner) Wait() {
	// TODO: self-implemented event loop
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

func (r *JsRunner) RunFile(path string) (goja.Value, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return r.RunScript(filepath.Base(path), string(b))
}

func (r *JsRunner) RunScript(name, src string) (result goja.Value, err error) {
	if r.running {
		ch := make(chan struct{})
		r.RunOnLoop(func(vm *goja.Runtime) {
			result, err = vm.RunScript(name, src)
			close(ch)
		})
		<-ch
	} else {
		result, err = r.vmUnsafe.RunScript(name, src)
	}
	return
}

// should be called from JS function
func (r *JsRunner) StartGoroutineTask(f func() (any, error)) goja.Value {
	r.wg.Add(1)
	promise, resolve, reject := r.vmUnsafe.NewPromise()
	go func() {
		result, err := f()
		r.RunOnLoop(func(*goja.Runtime) {
			if err == nil {
				resolve(result)
			} else {
				reject(err)
			}
			r.wg.Done()
		})
	}()
	return r.vmUnsafe.ToValue(promise)
}

// should be called from Go function
func (r *JsRunner) Await(value goja.Value) (result, err goja.Value) {
	if p, ok := value.Export().(*goja.Promise); ok {
		ch := make(chan struct{})
		if !r.RunOnLoop(func(vm *goja.Runtime) {
			switch p.State() {
			case goja.PromiseStateRejected:
				err = p.Result()
				close(ch)
			case goja.PromiseStateFulfilled:
				result = p.Result()
				close(ch)
			default:
				if f, ok := goja.AssertFunction(value.ToObject(vm).Get("then")); ok {
					f(value, vm.ToValue(func(r goja.Value) {
						result = r
						close(ch)
					}), vm.ToValue(func(r goja.Value) {
						err = r
						close(ch)
					}))
				}
			}
		}) {
			// terminated
			close(ch)
		}
		<-ch
	} else {
		result = value
	}
	return
}
