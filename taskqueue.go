package main

import (
	"context"
	"sync"
)

type Task interface {
	Run()
}

type TaskQueue struct {
	semaphoreCh chan struct{}
	taskCh      chan Task
	wg          sync.WaitGroup
	mutex       sync.RWMutex
	entries     map[string]*QueueEntry
}

func NewTaskQueue(parallel int, queueLen int, start bool) *TaskQueue {
	d := &TaskQueue{
		semaphoreCh: make(chan struct{}, parallel),
		taskCh:      make(chan Task, queueLen),
		entries:     map[string]*QueueEntry{},
	}
	if start {
		d.Start(context.Background())
	}
	return d
}

func (d *TaskQueue) Start(ctx context.Context) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case d.semaphoreCh <- struct{}{}:
				select {
				case <-ctx.Done():
					<-d.semaphoreCh
					return
				case task := <-d.taskCh:
					d.wg.Add(1)

					go func() {
						defer d.wg.Done()
						defer func() { <-d.semaphoreCh }()
						task.Run()
					}()
				}
			}
		}
	}()
}

func (d *TaskQueue) Wait() {
	d.wg.Wait()
}

func (d *TaskQueue) PostTask(t Task, block bool) bool {
	if block {
		d.taskCh <- t
	} else {
		select {
		case d.taskCh <- t:
		default:
			return false
		}
	}
	return true
}

type QueueEntry struct {
	task Task
	id   string
	done chan struct{}
	d    *TaskQueue
}

func (t *QueueEntry) ID() string {
	return t.id
}

func (t *QueueEntry) Done() <-chan struct{} {
	return t.done
}

func (t *QueueEntry) Run() {
	defer t.finish()
	t.task.Run()
}

func (t *QueueEntry) finish() {
	close(t.done)

	t.d.mutex.Lock()
	defer t.d.mutex.Unlock()
	delete(t.d.entries, t.ID())
}

func (d *TaskQueue) addTaskState(task Task, id string, block bool) (*QueueEntry, bool) {
	var once sync.Once
	d.mutex.Lock()
	defer once.Do(func() { d.mutex.Unlock() })

	if t, exists := d.entries[id]; exists {
		return t, false
	}
	ts := &QueueEntry{task, id, make(chan struct{}), d}
	if id != "" {
		d.entries[id] = ts
	}
	if !block {
		if !d.PostTask(ts, block) {
			delete(d.entries, id)
			return nil, false
		}
	} else {
		once.Do(func() { d.mutex.Unlock() })
		d.PostTask(ts, block)
	}
	return ts, true
}

func (d *TaskQueue) PostWithId(task Task, id string) (*QueueEntry, bool) {
	return d.addTaskState(task, id, true)
}

func (d *TaskQueue) TryPostWithId(task Task, id string) (*QueueEntry, bool) {
	return d.addTaskState(task, id, false)
}

type taskFunc func()

func (f taskFunc) Run() {
	f()
}

func (d *TaskQueue) PostFunc(taskFn func(), id string) (*QueueEntry, bool) {
	return d.PostWithId(taskFunc(taskFn), id)
}

func (d *TaskQueue) TryPostFunc(taskFn func(), id string) (*QueueEntry, bool) {
	return d.TryPostWithId(taskFunc(taskFn), id)
}
