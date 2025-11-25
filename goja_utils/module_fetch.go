package goja_utils

import (
	"fmt"
	"io"
	"net/http"

	"github.com/dop251/goja"
)

type JsMap map[string]any

func (o JsMap) GetString(name, def string) string {
	if o == nil {
		return def
	}
	if v, ok := o[name]; ok {
		return fmt.Sprint(v)
	}
	return def
}

func fetch(r *JsRunner) any {
	return func(url string, options JsMap) any {
		method := options.GetString("method", "GET")
		req, err := http.NewRequest(method, url, nil)
		if h, ok := options["headers"].(map[string]any); ok {
			for k, v := range h {
				req.Header.Add(k, fmt.Sprint(v))
			}
		}
		return r.GoAsync(func() (any, error) {
			if err != nil {
				return nil, err
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			resHeaders := map[string]any{}
			for k, v := range resp.Header {
				resHeaders[k] = v
			}
			return map[string]any{"body": body, "text": func() string { return string(body) }, "status": resp.StatusCode, "headers": resHeaders}, nil
		})
	}
}

func EnableFetch(vm *goja.Runtime) {
	vm.Set("fetch", fetch(GetJsRunner(vm)))
}
