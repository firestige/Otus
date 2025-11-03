package scheduler

import (
	"sync"
	"sync/atomic"

	"firestige.xyz/otus/internal/otus/pipeline"
)

type Scheduler struct {
	jobs      map[int]*Job
	nextJobID int64 // 原子计数器，确保 ID 单调递增
	mu        sync.RWMutex
}

var (
	instance *Scheduler
	once     sync.Once
)

func GetScheduler() *Scheduler {
	once.Do(func() {
		instance = &Scheduler{
			jobs:      make(map[int]*Job),
			nextJobID: 0,
		}
	})
	return instance
}

func (s *Scheduler) AddJob(cfg *pipeline.PipelineConfig) int {
	// 原子地获取并递增 ID
	jobID := int(atomic.AddInt64(&s.nextJobID, 1))

	job := NewJob(jobID, cfg.Name, cfg)
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	return jobID
}

func (s *Scheduler) RemoveJob(jobID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job, exists := s.jobs[jobID]; exists {
		job.Stop()
		delete(s.jobs, jobID)
		return true
	}
	return false
}

func (s *Scheduler) GetJob(jobID int) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, exists := s.jobs[jobID]
	return job, exists
}
