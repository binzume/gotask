package main

import (
	"os"
	"sync"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

type SchedulerEntry struct {
	TaskID string            `json:"taskId"`
	Spec   string            `json:"spec"`
	Params map[string]string `json:"params" yaml:"params,omitempty"`
	cronid cron.EntryID
}

type Scheduler struct {
	schedules []*SchedulerEntry
	manager   *Manager
	runner    *Runner
	conf      string
	c         *cron.Cron
	mutex     sync.RWMutex
}

func NewScheduler(manager *Manager, runner *Runner, conf string) *Scheduler {
	return &Scheduler{runner: runner, manager: manager, conf: conf, c: cron.New()}
}
func (s *Scheduler) Start() error {
	err := s.Reload()
	s.c.Start()
	return err
}

func (s *Scheduler) Reload() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	bytes, err := os.ReadFile(s.conf)
	if err != nil {
		return err
	}
	var schedules []*SchedulerEntry
	if err = yaml.Unmarshal(bytes, &schedules); err != nil {
		return err
	}

	// unregister all
	for _, ent := range s.schedules {
		s.unregister(ent)
	}

	s.schedules = schedules
	for _, ent := range s.schedules {
		s.register(ent)
	}
	return nil
}
func (s *Scheduler) Save() error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.save()
}

func (s *Scheduler) save() error {
	bytes, err := yaml.Marshal(s.schedules)
	if err != nil {
		return err
	}
	f, err := os.Create(s.conf)
	if err != nil {
		return err
	}
	_, err = f.Write(bytes)
	return err
}

func (s *Scheduler) Schedules() []*SchedulerEntry {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	schedules := make([]*SchedulerEntry, len(s.schedules))
	copy(schedules, s.schedules)
	return schedules
}

func (s *Scheduler) GetSchedule(taskId string) *SchedulerEntry {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for _, sch := range s.schedules {
		if sch.TaskID == taskId {
			return sch
		}
	}
	return nil
}

func (s *Scheduler) Set(taskID string, schedule string) error {
	s.Remove(taskID)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if schedule != "" {
		ent := &SchedulerEntry{TaskID: taskID, Spec: schedule}
		err := s.register(ent)
		if err != nil {
			return err
		}
		s.schedules = append(s.schedules, ent)
		err = s.save()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) Remove(taskID string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for i, ent := range s.schedules {
		if ent.TaskID == taskID {
			s.unregister(ent)
			s.schedules = append(s.schedules[0:i], s.schedules[i+1:]...)
			err := s.save()
			return err == nil
		}
	}
	return false
}

func (s *Scheduler) register(ent *SchedulerEntry) error {
	if ent.cronid != 0 {
		return nil
	}
	cronid, err := s.c.AddFunc(ent.Spec, func() {
		task, err := s.manager.Load(ent.TaskID)
		if err != nil {
			return
		}
		s.runner.Start(task, ent.Params)
	})
	ent.cronid = cronid
	return err
}

func (s *Scheduler) unregister(ent *SchedulerEntry) {
	if ent.cronid == 0 {
		return
	}
	s.c.Remove(ent.cronid)
	ent.cronid = 0
}
