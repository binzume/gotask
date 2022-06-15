package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type RunnerConfig struct {
	Tags      []string
	Queues    map[string]interface{}
	QueueSize int
	LogDir    string
	Parallel  int
}

func (conf *RunnerConfig) FillDefault() *RunnerConfig {
	if conf == nil {
		conf = &RunnerConfig{}
	}
	if conf.LogDir == "" {
		conf.LogDir = "./logs"
	}
	if conf.QueueSize == 0 {
		conf.QueueSize = 100
	}
	if conf.Parallel == 0 {
		conf.Parallel = 8
	}
	return conf
}

type LogEntry struct {
	TaskID string     `json:"taskId"`
	RunID  int64      `json:"runId"`
	Task   *TaskState `json:"task"`

	Params map[string]string `json:"params,omitempty"`
}

type TaskState struct {
	Name    string   `json:"name"`
	Depends []string `json:"depends,omitempty"`

	Steps []*TaskState `json:"steps,omitempty"`

	Status     string `json:"status"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
	LogFile    string `json:"logFile,omitempty"`
	Message    string `json:"message,omitempty"`
}

type runState struct {
	task   *TaskState
	config *TaskConfig
	done   chan struct{}
	cancel context.CancelFunc

	log *LogEntry
}

func (t *runState) wait() {
	<-t.done
}

type Runner struct {
	runnings    []*runState
	queue       *TaskQueue
	mutex       sync.RWMutex
	recentLimit int
	logDir      string
}

func NewRunner(conf *RunnerConfig) *Runner {
	conf = conf.FillDefault()
	queue := NewTaskQueue(conf.Parallel, conf.QueueSize, false)
	queue.Start(context.Background())
	return &Runner{
		queue:       queue,
		logDir:      conf.LogDir,
		recentLimit: 100,
	}
}

func (r *Runner) LogDir() string {
	return r.logDir
}

func (r *Runner) Start(config *TaskConfig, params map[string]string) *LogEntry {
	// TODO: validate task graph before start.
	log := &LogEntry{
		TaskID: config.TaskID,
		RunID:  time.Now().UnixMilli(),
		Task:   NewTaskLog(config),
		Params: params,
	}
	state := r.startInternal(context.Background(), config, log, log.Task)
	r.addTask(state)
	go func() {
		state.wait()
		r.finishTask(state)
	}()
	return log
}

func (r *Runner) startInternal(ctx context.Context, config *TaskConfig, logEnt *LogEntry, log *TaskState) *runState {
	ctx2, cancel := context.WithCancel(ctx)
	state := &runState{
		task:   log,
		config: config,
		done:   make(chan struct{}),
		cancel: cancel,
		log:    logEnt,
	}
	for _, t := range config.Steps {
		state.task.Steps = append(state.task.Steps, NewTaskLog(t))
	}
	for k, v := range logEnt.Params {
		if config.Variables != nil && config.Variables[k] != nil {
			config.Variables[k] = v
		}
	}

	state.task.Status = "queued"
	go func() {
		state.run(ctx2, r)
	}()
	return state
}

func (r *Runner) Stop(taskID string, runID int64) bool {
	state := r.getRunningTask(taskID, runID)
	if state == nil {
		return false
	}
	state.cancel()
	return true
}

func (r *Runner) Wait(taskID string, runID int64) bool {
	state := r.getRunningTask(taskID, runID)
	if state == nil {
		return false
	}
	state.wait()
	return true
}

func (r *Runner) RunningTaks(taskID string) []*LogEntry {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	var tasks []*LogEntry
	for _, t := range r.runnings {
		if t.log.TaskID == taskID {
			tasks = append(tasks, t.log)
		}
	}
	return tasks
}

func (r *Runner) GetHistory(taskID string, limit int) []*LogEntry {
	log := r.RunningTaks(taskID)

	f, err := os.Open(filepath.Join(r.logDir, taskID, "task.log"))
	if err != nil {
		// no log file
		return log
	}
	defer f.Close()

	// TODO: Read last N lines.
	var log2 []*LogEntry
	f.Seek(-65536, io.SeekEnd)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		var ent LogEntry
		if json.Unmarshal(line, &ent) == nil && ent.Task != nil {
			log2 = append(log2, &ent)
		}
	}
	for i := range log2 {
		log = append(log, log2[len(log2)-i-1])
		if len(log) >= limit {
			break
		}
	}
	return log
}

func (state *runState) tryStartSteps(r *Runner, ctx context.Context, steps map[string]*TaskState, stepsState map[string]bool, done chan struct{}) int {
	startCount := 0
	for _, child := range state.config.Steps {
		if _, ok := stepsState[child.Name]; ok {
			// already started
			continue
		}
		ready := true
		for _, d := range child.Depends {
			dt := steps[d]
			if dt == nil || dt.Status != "success" {
				ready = false
			}
		}
		if !ready {
			continue
		}

		if child.Dir == "" {
			child.Dir = state.config.Dir
		}
		clog := steps[child.Name]

		cs := r.startInternal(ctx, child, state.log, clog)
		name := child.Name
		stepsState[name] = false
		startCount++
		go func() {
			cs.wait()
			done <- struct{}{}
		}()
	}
	return startCount
}

func (state *runState) run(ctx context.Context, r *Runner) {
	defer close(state.done)

	steps := map[string]*TaskState{}
	for _, t := range state.task.Steps {
		steps[t.Name] = t
	}

	if len(steps) > 0 {
		state.task.StartedAt = time.Now().UnixMilli()
		state.task.Status = "running"
	}

	runnings := 0
	stepsState := map[string]bool{}
	stepDone := make(chan struct{})
	for {
		runnings += state.tryStartSteps(r, ctx, steps, stepsState, stepDone)
		if runnings == 0 {
			break
		}
		<-stepDone
		runnings--
	}
	select {
	case <-ctx.Done():
		state.task.FinishedAt = time.Now().UnixMilli()
		state.task.Status = "canceled"
		return
	default:
	}
	ok := true
	for _, d := range state.task.Steps {
		if d.Status != "success" {
			ok = false
		}
	}
	if !ok {
		state.task.FinishedAt = time.Now().UnixMilli()
		state.task.Status = "failed"
		state.task.Message = "sub tasks are not completed"
		return
	}

	tid := state.log.TaskID + ":" + state.task.Name + fmt.Sprintf(".%d", time.Now().UnixMilli())
	queueState, _ := r.queue.TryPostFunc(func() {
		if state.task.StartedAt == 0 {
			state.task.StartedAt = time.Now().UnixMilli()
			state.task.Status = "running"
		}
		if state.config.Command == "" {
			state.task.FinishedAt = time.Now().UnixMilli()
			state.task.Status = "success"
			return
		}
		state.task.LogFile = fmt.Sprintf("%s/%d_%s.log", state.log.TaskID, state.log.RunID, state.task.Name)
		logPath := filepath.Join(r.logDir, state.task.LogFile)
		_ = os.MkdirAll(filepath.Dir(logPath), os.ModePerm)
		cmd := exec.CommandContext(ctx, "bash", "-c", state.config.Command)
		cmd.Dir = state.config.Dir
		cmd.Env = os.Environ()
		log, _ := os.Create(logPath)
		if log != nil {
			cmd.Stdout = log
			cmd.Stderr = log
			defer log.Close()
		}
		for n, v := range state.config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", n, v))
		}
		for n, v := range state.config.Variables {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", n, v))
		}
		_ = cmd.Run()
		state.task.FinishedAt = time.Now().UnixMilli()

		code := cmd.ProcessState.ExitCode()
		if code != 0 && code == state.config.CanceledExitCode {
			state.task.Status = "canceled"
		} else if code != 0 {
			select {
			case <-ctx.Done():
				state.task.Status = "canceled"
				return
			default:
			}
			state.task.Status = "failed"
			state.task.Message = "command exited with code " + fmt.Sprint(code)
		} else {
			state.task.Status = "success"
		}
	}, tid)
	if queueState == nil {
		state.task.Status = "failed"
		state.task.Message = "failed to enqueue"
		return
	}
	<-queueState.Done()
}

func NewTaskLog(task *TaskConfig) *TaskState {
	return &TaskState{
		Name:    task.Name,
		Depends: task.Depends,
	}
}

func (r *Runner) addTask(state *runState) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.runnings = append(r.runnings, state)
}

func (r *Runner) appendLog(log *LogEntry) {
	json, err := json.Marshal(log)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(r.logDir, log.TaskID, "task.log"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(json)
	f.WriteString("\n")
}

func (r *Runner) getRunningTask(taskID string, runID int64) *runState {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	for _, t := range r.runnings {
		if t.log.RunID == runID && t.log.TaskID == taskID {
			return t
		}
	}
	return nil
}

func (r *Runner) finishTask(state *runState) {
	go func() {
		state.wait()
		r.appendLog(state.log)
	}()

	r.mutex.Lock()
	defer r.mutex.Unlock()
	for i, t := range r.runnings {
		if t == state {
			r.runnings = append(r.runnings[:i], r.runnings[i+1:]...)
			break
		}
	}
}
