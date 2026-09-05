package logging

import (
	"testing"
	"time"
)

func TestTimersStartStop(t *testing.T) {
	var tm Timers
	tm.Start(TimerTest)
	time.Sleep(2 * time.Millisecond)
	tm.Stop(TimerTest)

	if tm.Get(TimerTest) <= 0 {
		t.Fatalf("expected positive elapsed time, got %v", tm.Get(TimerTest))
	}
}

func TestTimersStopWithoutStartIsNoop(t *testing.T) {
	var tm Timers
	tm.Stop(TimerTest)
	if tm.Get(TimerTest) != 0 {
		t.Fatalf("expected zero elapsed time, got %v", tm.Get(TimerTest))
	}
}

func TestTimersReset(t *testing.T) {
	var tm Timers
	tm.Start(TimerTest)
	time.Sleep(time.Millisecond)
	tm.Stop(TimerTest)
	tm.Reset(TimerTest)
	if tm.Get(TimerTest) != 0 {
		t.Fatalf("expected zero elapsed time after reset, got %v", tm.Get(TimerTest))
	}
}

func TestTimersStopOnce(t *testing.T) {
	var tm Timers
	tm.Start(TimerTotal)
	time.Sleep(2 * time.Millisecond)

	tm.StopOnce(TimerStartup, TimerTotal)
	first := tm.Get(TimerStartup)
	if first <= 0 {
		t.Fatalf("expected positive elapsed time, got %v", first)
	}

	time.Sleep(2 * time.Millisecond)
	tm.StopOnce(TimerStartup, TimerTotal)
	if tm.Get(TimerStartup) != first {
		t.Fatalf("expected StopOnce to be a no-op the second time, got %v want %v", tm.Get(TimerStartup), first)
	}
}
