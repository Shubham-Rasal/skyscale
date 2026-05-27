package rlevents

import (
	"sync"
	"time"
)

// Event is a single RL pipeline log entry visible in the dashboard and server logs.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	RunID     string    `json:"run_id"`
	Component string    `json:"component"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// Log stores recent events per run in memory.
type Log struct {
	mu        sync.RWMutex
	maxPerRun int
	events    map[string][]Event
}

// Default is the shared in-process event log.
var Default = New(300)

func New(maxPerRun int) *Log {
	if maxPerRun <= 0 {
		maxPerRun = 200
	}
	return &Log{maxPerRun: maxPerRun, events: make(map[string][]Event)}
}

func (l *Log) Record(runID, component, level, message string) {
	if runID == "" || message == "" {
		return
	}
	if level == "" {
		level = "info"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ev := Event{
		Timestamp: time.Now().UTC(),
		RunID:     runID,
		Component: component,
		Level:     level,
		Message:   message,
	}
	list := append(l.events[runID], ev)
	if len(list) > l.maxPerRun {
		list = list[len(list)-l.maxPerRun:]
	}
	l.events[runID] = list
}

func (l *Log) List(runID string, limit int) []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	list := l.events[runID]
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	if limit == 0 {
		return nil
	}
	out := make([]Event, limit)
	copy(out, list[len(list)-limit:])
	return out
}
