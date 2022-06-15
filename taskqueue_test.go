package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestTask_Func(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := NewTaskQueue(4, 10, false)
	scheduler.Start(ctx)

	var count uint32

	ts1, _ := scheduler.PostFunc(func() { atomic.AddUint32(&count, 1) }, "")
	ts2, _ := scheduler.PostFunc(func() { atomic.AddUint32(&count, 1) }, "")
	ts3, _ := scheduler.PostFunc(func() { atomic.AddUint32(&count, 1) }, "")

	<-ts1.Done()
	<-ts2.Done()
	<-ts3.Done()

	if count != 3 {
		t.Error("count != 3")
	}
}

func TestTask_TaskId(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := NewTaskQueue(4, 10, false)
	scheduler.Start(ctx)

	done := make(chan struct{})
	var countA uint32
	var countB uint32

	ts1, _ := scheduler.TryPostFunc(func() { atomic.AddUint32(&countA, 1); <-done }, "TaskA")
	ts2, _ := scheduler.TryPostFunc(func() { atomic.AddUint32(&countA, 1); <-done }, "TaskA")
	ts3, _ := scheduler.TryPostFunc(func() { atomic.AddUint32(&countB, 1); <-done }, "TaskB")

	close(done)
	<-ts1.Done()
	<-ts2.Done()
	<-ts3.Done()

	if countA != 1 {
		t.Error("countA != 1")
	}
	if countB != 1 {
		t.Error("countB != 1")
	}
}

func TestTask_Buffer0(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// bufferLen = 0
	scheduler := NewTaskQueue(1, 0, false)
	scheduler.Start(ctx)

	time.Sleep(10 * time.Millisecond)
	ts1, _ := scheduler.TryPostFunc(func() { <-ctx.Done() }, "")
	if ts1 == nil {
		t.Fatal("ts1 == nil")
	}

	time.Sleep(10 * time.Millisecond)
	ts2, _ := scheduler.TryPostFunc(func() {}, "")
	if ts2 != nil {
		t.Fatal("ts2 != nil")
	}
}

func TestTask_WaitFInish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	scheduler := NewTaskQueue(4, 10, false)
	scheduler.Start(ctx)

	started := make(chan struct{})
	finished := false
	scheduler.TryPostFunc(func() {
		close(started)
		time.Sleep(50 * time.Millisecond)
		finished = true
	}, "")
	<-started

	cancel()
	scheduler.Wait()
	if !finished {
		t.Error("started task is not finished")
	}
}
