package goja_utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

func makeFetch(vm *goja.Runtime) any {
	r := GetTaskQueue(vm)
	return func(url string, options JsMap) any {
		method := options.GetString("method", "GET")
		var body io.Reader
		if _, ok := options["body"]; ok {
			body = strings.NewReader(options.GetString("body", ""))
		}
		req, err := http.NewRequest(method, url, body)
		if h, ok := options["headers"].(map[string]any); ok {
			for k, v := range h {
				req.Header.Add(k, fmt.Sprint(v))
			}
		}
		return r.StartGoroutineTask(func() (any, error) {
			if err != nil {
				return nil, err
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}

			fmt.Println("fetch start")
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			resHeaders := map[string]any{}
			for k, v := range resp.Header {
				resHeaders[k] = v
			}
			resp.Location()
			fmt.Println("fetch ok")
			return map[string]any{
				"ok":    resp.StatusCode >= 200 && resp.StatusCode < 300,
				"text":  func() string { return string(body) },
				"bytes": func() any { return body },
				"json": func() any {
					data := map[string]any{}
					_ = json.Unmarshal(body, &data)
					return data
				},
				"arrayBuffer": func() any { return vm.NewArrayBuffer(body) },
				"status":      resp.StatusCode,
				"statusText":  resp.Status,
				"url":         resp.Request.URL.String(),
				"headers":     resHeaders}, nil
		})
	}
}

func EnableFetch(vm *goja.Runtime) {
	vm.Set("fetch", makeFetch(vm))
}
