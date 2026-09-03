package logging

import "time"

// TimerID identifies one of the fixed set of elapsed-time measurements
// taken during a scan/match/install cycle.
type TimerID int

const (
	TimerTotal TimerID = iota
	TimerStartup
	TimerIndexes
	TimerDeviceScan
	TimerChkUpdate
	TimerIndexSave
	TimerIndexPrint
	TimerSysInfo
	TimerMatcher
	TimerTest
	timerCount
)

// Timers tracks elapsed time for a fixed set of named phases. Unlike the
// original tick-count based Timers_t, it uses time.Time/time.Duration
// directly, so there is no wraparound to guard against.
type Timers struct {
	started [timerCount]time.Time
	elapsed [timerCount]time.Duration
}

// Start marks the beginning of a timed phase.
func (t *Timers) Start(id TimerID) {
	t.started[id] = time.Now()
}

// Stop records the elapsed time since Start, if Start was called.
func (t *Timers) Stop(id TimerID) {
	if !t.started[id].IsZero() {
		t.elapsed[id] = time.Since(t.started[id])
	}
}

// Reset clears both the start time and the recorded elapsed time.
func (t *Timers) Reset(id TimerID) {
	t.started[id] = time.Time{}
	t.elapsed[id] = 0
}

// StopOnce records the time elapsed since ref's Start, but only if id
// has no recorded elapsed time yet.
func (t *Timers) StopOnce(id, ref TimerID) {
	if t.elapsed[id] == 0 && !t.started[ref].IsZero() {
		t.elapsed[id] = time.Since(t.started[ref])
	}
}

// Get returns the recorded elapsed time for id.
func (t *Timers) Get(id TimerID) time.Duration {
	return t.elapsed[id]
}
