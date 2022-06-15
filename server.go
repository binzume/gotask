package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var manager = NewManager(nil)
var runner = NewRunner(nil)

func responseJson(w http.ResponseWriter, res interface{}) {
	json, err := json.Marshal(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(json)
}

func handlePostTask(w http.ResponseWriter, task *TaskConfig, vars url.Values) {
	res := struct {
		TaskID string `json:"taskId"`
		RunID  int64  `json:"runId"`
		Ok     bool   `json:"ok"`
	}{}
	res.TaskID = task.TaskID
	if vars.Get("action") == "stop" {
		id, _ := strconv.ParseInt(vars.Get("runId"), 10, 64)
		res.Ok = runner.Stop(task.TaskID, id)
		res.RunID = id
	} else {
		params := map[string]string{}
		for k, v := range vars {
			if strings.HasPrefix(k, "VARS.") {
				params[k[5:]] = v[0]
			}
		}
		ent := runner.Start(task, params)
		res.RunID = ent.RunID
		res.Ok = true
	}
	responseJson(w, &res)
}

func handler(w http.ResponseWriter, r *http.Request) {
	taskID := strings.SplitN(r.URL.Path, "/", 2)[0]
	if taskID == "" {
		responseJson(w, manager.Tasks())
		return
	}
	task, err := manager.Load(taskID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method == "POST" {
		r.ParseMultipartForm(4096)
		handlePostTask(w, task, r.PostForm)
		return
	}

	res := struct {
		Task   *TaskConfig `json:"task"`
		Recent []*LogEntry `json:"recent"`
	}{
		Task:   task,
		Recent: runner.GetHistory(taskID, 50),
	}
	responseJson(w, &res)
}

func main() {
	port := os.Getenv("GOTASK_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("GOTASK_HTTP_HOST")

	http.Handle("/", http.FileServer(http.Dir("static")))
	http.Handle("/tasks/", http.StripPrefix("/tasks/", http.HandlerFunc(handler)))
	http.Handle("/tasklogs/", http.StripPrefix("/tasklogs/", http.FileServer(http.Dir(runner.LogDir()))))
	http.ListenAndServe(host+":"+port, nil)
}
