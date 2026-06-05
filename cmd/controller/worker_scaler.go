package main

import "time"

type WorkerScaleConfig struct {
	MinCount                int
	MaxCount                int
	CountPerStart           int
	MinElapsedBetweenStarts time.Duration
}

type WorkerScaleState struct {
	StartedWorkers      int
	LastStart           time.Time
	WaitingForClaim     bool
	AssignedAtLastStart int
}

func (s *WorkerScaleState) PlanStarts(now time.Time, pending int, assigned int, cfg WorkerScaleConfig) int {
	s.confirmClaim(assigned)

	if pending <= 0 || cfg.MaxCount <= 0 {
		return 0
	}

	countPerStart := cfg.CountPerStart
	if countPerStart <= 0 {
		countPerStart = 1
	}

	availableCapacity := cfg.MaxCount - s.StartedWorkers
	if availableCapacity <= 0 {
		return 0
	}

	usefulCapacity := minInt(availableCapacity, pending)
	if usefulCapacity <= 0 {
		return 0
	}

	if s.StartedWorkers < cfg.MinCount {
		minGap := cfg.MinCount - s.StartedWorkers
		return minInt(usefulCapacity, minInt(countPerStart, minGap))
	}

	if s.WaitingForClaim {
		return 0
	}

	if !s.LastStart.IsZero() && now.Sub(s.LastStart) < cfg.MinElapsedBetweenStarts {
		return 0
	}

	return minInt(usefulCapacity, countPerStart)
}

func (s *WorkerScaleState) RecordStart(now time.Time, count int, assigned int) {
	if count <= 0 {
		return
	}

	s.StartedWorkers += count
	s.LastStart = now
	s.WaitingForClaim = true
	s.AssignedAtLastStart = assigned
}

func (s *WorkerScaleState) confirmClaim(assigned int) {
	if s.WaitingForClaim && assigned > s.AssignedAtLastStart {
		s.WaitingForClaim = false
	}
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
