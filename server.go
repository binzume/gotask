package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var manager = NewManager(nil)
var runner = NewRunner(nil)
var scheduler *Scheduler

//go:embed static/*
var staticFS embed.FS

func responseJson(w http.ResponseWriter, res interface{}) {
	json, err := json.Marshal(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(json)
}

func handlePostTask(ctx context.Context, w http.ResponseWriter, task *TaskConfig, vars url.Values) {
	res := struct {
		TaskID  string `json:"taskId"`
		RunID   int64  `json:"runId"`
		Ok      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}{}
	res.TaskID = task.TaskID
	action := vars.Get("action")
	if action == "stop" {
		id, _ := strconv.ParseInt(vars.Get("runId"), 10, 64)
		res.Ok = runner.Stop(task.TaskID, id)
		res.RunID = id
	} else if action == "invoke" {
		params := map[string]any{}
		for k, v := range task.Variables {
			params[k] = v
		}
		for k, v := range vars {
			if strings.HasPrefix(k, "VARS.") {
				params[k[5:]] = v[0]
			} else if strings.HasPrefix(k, "PARAMS.") {
				params[k[7:]] = v[0]
			}
		}
		fmt.Println(params)
		// TODO: lock
		// runner.appendLog(&LogEntry{TaskID: task.TaskID})
		r := task.Run(ctx, params, nil)
		if r.Success && r.Result != nil {
			if body, ok := r.Result["body"].(string); ok {
				if headers, ok := r.Result["headers"].(map[string]any); ok {
					for k, v := range headers {
						w.Header().Set(k, fmt.Sprint(v))
					}
				} else {
					w.Header().Set("Content-Type", "text/plain")
				}
				if status, ok := r.Result["statusCode"].(int); ok {
					w.WriteHeader(status)
				}
				w.Write([]byte(body))
				return
			}
			responseJson(w, r.Result)
			return
		}
		res.Message = r.Message
		res.Ok = r.Success
	} else {
		params := map[string]any{}
		for k, v := range vars {
			if strings.HasPrefix(k, "VARS.") {
				params[k[5:]] = v[0]
			} else if strings.HasPrefix(k, "PARAMS.") {
				params[k[7:]] = v[0]
			}
		}
		ent, err := runner.Start(task, params)
		if err == nil {
			res.RunID = ent.RunID
			res.Ok = true
		} else {
			res.Ok = false
		}
	}
	responseJson(w, &res)
}

func taskHandler(w http.ResponseWriter, r *http.Request) {
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
		handlePostTask(r.Context(), w, task, r.PostForm)
		return
	}

	res := struct {
		Task     *TaskConfig     `json:"task"`
		Recent   []*LogEntry     `json:"recent"`
		Schedule *SchedulerEntry `json:"schedule,omitempty"`
	}{
		Task:     task,
		Recent:   runner.GetHistory(taskID, 50),
		Schedule: scheduler.GetSchedule(taskID),
	}
	responseJson(w, &res)
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		r.ParseMultipartForm(4096)
		taskID := r.PostForm.Get("taskId")
		schedule := r.PostForm.Get("schedule")
		_, err := manager.Load(taskID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if schedule == "" {
			scheduler.Remove(taskID)
		} else {
			err := scheduler.Set(taskID, schedule)
			if err != nil {
				http.Error(w, "invalid schedule", http.StatusBadRequest)
				return
			}
		}
	}
	responseJson(w, scheduler.Schedules())
}

func main() {
	fixedtz := os.Getenv("GOTASK_FIXED_TZ") // ex: JST-9
	if p := strings.LastIndexAny(fixedtz, "+-"); p >= 0 {
		offset, _ := strconv.Atoi(fixedtz[p:])
		time.Local = time.FixedZone(fixedtz, -offset*3600)
	}
	scheduler = NewScheduler(manager, runner, "tasks/_schedules.yaml")
	err := scheduler.Start()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Println(err)
	}

	port := os.Getenv("GOTASK_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("GOTASK_HTTP_HOST")

	staticDir := os.Getenv("GOTASK_HTTP_STATIC_DIR")
	if staticDir != "" {
		http.Handle("/", http.FileServer(http.Dir(staticDir)))
	} else {
		static, _ := fs.Sub(staticFS, "static")
		http.Handle("/", http.FileServer(http.FS(static)))
	}
	http.Handle("/tasks/", http.StripPrefix("/tasks/", http.HandlerFunc(taskHandler)))
	http.Handle("/tasklogs/", http.StripPrefix("/tasklogs/", http.FileServer(http.Dir(runner.LogDir()))))
	http.Handle("/schedules/", http.StripPrefix("/schedules/", http.HandlerFunc(scheduleHandler)))
	http.ListenAndServe(host+":"+port, nil)
}
