package goja_utils

import (
	"io"
	"os"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

func ReadFileSync(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(data)
}

func WriteFileSync(path, text string) error {
	f, err := os.Create(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func AppendFileSync(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND, os.ModePerm)
	if err != nil {
		return nil
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func ReadFileAsync(r *JsRunner) any {
	return func(name string) goja.Value {
		return r.GoAsync(func() (any, error) {
			f, err := os.Open(name)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			return string(data), err
		})
	}
}

func WriteFileAsync(r *JsRunner) any {
	return func(name, text string) goja.Value {
		return r.GoAsync(func() (any, error) {
			f, err := os.Create(name)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			_, err = f.WriteString(text)
			return nil, err
		})
	}
}

func SetupFsPromises(runtime *goja.Runtime, o *goja.Object) {
	if r := GetJsRunner(runtime); r != nil {
		o.Set("readFile", ReadFileAsync(r))
		o.Set("writeFile", WriteFileAsync(r))
	}
}

func RequireFs(runtime *goja.Runtime, module *goja.Object) {
	o := module.Get("exports").(*goja.Object)
	o.Set("readFileSync", ReadFileSync)
	o.Set("appendFileSync", AppendFileSync)
	o.Set("writeFileSync", WriteFileSync)
	po := runtime.NewObject()
	SetupFsPromises(runtime, o)
	o.Set("promises", po)
}

func EnableFs(runtime *goja.Runtime) {
	runtime.Set("fs", require.Require(runtime, "fs"))
}

func init() {
	require.RegisterCoreModule("fs", RequireFs)
}
