package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type TaskConfig struct {
	Name             string `json:"name"`
	Description      string `json:"desc"`
	Runtime          string `json:"runtime"`
	Command          string `json:"command"`
	Env              map[string]string
	Variables        map[string]interface{}
	Dir              string
	Depends          []string `json:"depends"`
	CanceledExitCode int
	AllowParallel    bool
	DisableLog       bool `json:"disableLog"`

	Sequential bool
	Steps      []*TaskConfig `json:"steps"`

	TaskID string `json:"taskId"`
}

type TaskResult struct {
	Success  bool
	Canceled bool
	Result   map[string]any
	Message  string
}

func (conf *TaskConfig) FixDependencies() {
	for i, t := range conf.Steps {
		if conf.Sequential {
			if i > 0 {
				t.Depends = []string{conf.Steps[i-1].Name}
			} else {
				t.Depends = nil
			}
		}
		t.FixDependencies()
	}
}

func (c *TaskConfig) Run(ctx context.Context, params map[string]any, log io.Writer) *TaskResult {
	if c.Runtime == "js" {
		return RunJs(ctx, c, params, log)
	} else {
		return RunSh(ctx, c, params, log)
	}
}

type TaskListItem struct {
	TaskID string `json:"taskId"`
}

type ManagerConfig struct {
	TasksDir string
}

func (conf *ManagerConfig) FillDefault() *ManagerConfig {
	if conf == nil {
		conf = &ManagerConfig{}
	}
	if conf.TasksDir == "" {
		conf.TasksDir = "./tasks"
	}
	return conf
}

type Manager struct {
	tasksDir string
}

func NewManager(conf *ManagerConfig) *Manager {
	return &Manager{tasksDir: conf.FillDefault().TasksDir}
}

func (m *Manager) loadYAML(taskId string, task *TaskConfig) error {
	conf := filepath.Join(m.tasksDir, taskId+".yaml")
	bytes, err := os.ReadFile(conf)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bytes, &task)
}

func (m *Manager) loadSh(taskId string, task *TaskConfig) error {
	task.Name = taskId
	task.Sequential = true
	_, err := os.Stat(filepath.Join(m.tasksDir, taskId+".sh"))
	if err == nil {
		task.Command = "./" + taskId + ".sh"
	}
	_, err = os.Stat(filepath.Join(m.tasksDir, taskId+".1.sh"))
	if err == nil {
		for i := 1; ; i++ {
			var sub TaskConfig
			err := m.loadSh(fmt.Sprintf("%s.%d", taskId, i), &sub)
			if err != nil {
				break
			}
			task.Steps = append(task.Steps, &sub)
		}
	}
	if task.Command != "" || len(task.Steps) > 0 {
		return nil
	}
	return err
}

func (m *Manager) loadJs(taskId string, task *TaskConfig) error {
	task.Name = taskId
	_, err := os.Stat(filepath.Join(m.tasksDir, taskId+".js"))
	if err == nil {
		task.Runtime = "js"
		task.Command = "./" + taskId + ".js"
	}
	return err
}

func (m *Manager) Load(taskId string) (*TaskConfig, error) {
	var task TaskConfig
	task.Dir = m.tasksDir
	task.Name = taskId
	task.TaskID = taskId

	err := m.loadYAML(taskId, &task)
	if errors.Is(err, os.ErrNotExist) {
		err = m.loadSh(taskId, &task)
		if err == nil {
			task.Command = "./" + taskId + ".sh"
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		err = m.loadJs(taskId, &task)
		if err == nil {
			task.Command = "./" + taskId + ".js"
		}
	}

	if err != nil {
		return nil, err
	}
	task.FixDependencies()
	return &task, err
}

func (m *Manager) Tasks() []*TaskListItem {
	var tasks []*TaskListItem
	var exists = map[string]bool{}
	files, _ := os.ReadDir(m.tasksDir)
	for _, f := range files {
		name := f.Name()
		ext := filepath.Ext(name)
		if !f.Type().IsRegular() || ext != ".yaml" && ext != ".sh" || name[0] == '.' || name[0] == '_' {
			continue
		}
		taskID := name[0 : len(name)-len(ext)]
		if ext == ".sh" {
			if p := strings.Index(taskID, "."); p != -1 {
				taskID = taskID[0:p] // sub task
			}
		}
		if exists[taskID] {
			continue
		}
		exists[taskID] = true
		tasks = append(tasks, &TaskListItem{TaskID: taskID})
	}
	return tasks
}
