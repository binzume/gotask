package main

import (
	"embed"
	"encoding/json"
	"errors"
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
		handlePostTask(w, task, r.PostForm)
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
