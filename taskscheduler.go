package main

import (
	"encoding/json"
	"os"

	"github.com/robfig/cron/v3"
)

type SchedulerEntry struct {
	TaskID   string            `json:"taskId"`
	Schedule string            `json:"schedule"`
	Params   map[string]string `json:"params"`
	cronid   cron.EntryID
}

type Scheduler struct {
	schedules []*SchedulerEntry
	manager   *Manager
	runner    *Runner
	conf      string
	c         *cron.Cron
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
	bytes, err := os.ReadFile(s.conf)
	if err != nil {
		return err
	}
	var schedules []*SchedulerEntry
	if err = json.Unmarshal(bytes, &schedules); err == nil {
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
	bytes, err := json.Marshal(s.schedules)
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
	schedules := make([]*SchedulerEntry, len(s.schedules))
	copy(schedules, s.schedules)
	return schedules
}

func (s *Scheduler) Add(taskID, schedule string) {
	s.Remove(taskID)
	ent := &SchedulerEntry{TaskID: taskID, Schedule: schedule}
	s.schedules = append(s.schedules, ent)
	s.register(ent)
	s.Save()
}

func (s *Scheduler) Remove(taskID string) bool {
	for i, ent := range s.schedules {
		if ent.TaskID == taskID {
			s.unregister(ent)
			s.schedules = append(s.schedules[0:i], s.schedules[i+1:]...)
			err := s.Save()
			return err == nil
		}
	}
	return false
}

func (s *Scheduler) register(ent *SchedulerEntry) {
	if ent.cronid != 0 {
		return
	}
	ent.cronid, _ = s.c.AddFunc(ent.Schedule, func() {
		task, err := s.manager.Load(ent.TaskID)
		if err != nil {
			return
		}
		s.runner.Start(task, ent.Params)
	})
}

func (s *Scheduler) unregister(ent *SchedulerEntry) {
	if ent.cronid == 0 {
		return
	}
	s.c.Remove(ent.cronid)
	ent.cronid = 0
}
