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
var auth *AuthConfig

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

func getAccount(r *http.Request) (*AccountConfig, error) {
	token := r.Header.Get("Authorization")
	if strings.HasPrefix(token, "Bearer ") {
		return auth.VerifyToken(token[7:])
	}
	c, err := r.Cookie("GOTASKSESSION")
	if err != nil {
		return nil, err
	}
	return auth.VerifySession(c.Value)
}

func authHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.SessionSecret == "" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/tasks/") && r.Method == "POST" {
			next.ServeHTTP(w, r)
			return
		}
		a, err := getAccount(r)
		if err != nil {
			http.Error(w, `{"error":"auth"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "ACCOUNT", a)))
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}

	r.ParseMultipartForm(4096)
	password := r.PostForm.Get("password")
	user := r.PostForm.Get("user")
	a, err := auth.VerifyPassword(user, password)
	if err != nil {
		http.Redirect(w, r, "login.html", 302)
		return
	}

	c := &http.Cookie{
		Name:     "GOTASKSESSION",
		Value:    auth.CreateSessionString(a),
		MaxAge:   int(time.Hour.Seconds()) * 24 * 365,
		Secure:   r.TLS != nil,
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, ".", 302)
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
	// Check role
	if len(task.RequiredRole) > 0 {
		a, _ := ctx.Value("ACCOUNT").(*AccountConfig)
		if a == nil && params["TOKEN"] != nil {
			// TODO: remove this
			a, _ = auth.getAccountByToken(params["TOKEN"].(string))
		}
		if a == nil || !a.HasRole(task.RequiredRole) {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
	}
	if action == "stop" {
		id, _ := strconv.ParseInt(vars.Get("runId"), 10, 64)
		res.Ok = runner.Stop(task.TaskID, id)
		res.RunID = id
	} else if action == "invoke" {
		r, _ := runner.Invoke(ctx, task, params)
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

func runHttpServer() {
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
	http.Handle("/login", http.HandlerFunc(loginHandler))
	http.Handle("/tasks/", authHandler(http.StripPrefix("/tasks/", http.HandlerFunc(taskHandler))))
	http.Handle("/tasklogs/", authHandler(http.StripPrefix("/tasklogs/", http.FileServer(http.Dir(runner.LogDir())))))
	http.Handle("/schedules/", authHandler(http.StripPrefix("/schedules/", http.HandlerFunc(scheduleHandler))))
	http.ListenAndServe(host+":"+port, nil)
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

	auth, _ = LoadAuthConfigYAML("auth.yaml")

	runHttpServer()
}
