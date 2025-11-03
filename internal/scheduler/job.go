package scheduler

import (
	"context"
	"time"

	"firestige.xyz/otus/internal/otus/pipeline"
)

type Job struct {
	ID        int
	Name      string
	CreatedAt int64

	p      pipeline.Pipeline
	status string

	ctx    context.Context
	cancel context.CancelFunc
}

func NewJob(id int, name string, cfg *pipeline.PipelineConfig) *Job {
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now().UnixMilli(),
		p:         *pipeline.NewPipeline(cfg),
		ctx:       ctx,
		cancel:    cancel,
	}
	j.p.Init()
	return j
}

func (j *Job) String() string {
	return j.Name
}

func (j *Job) IDString() string {
	return string(rune(j.ID))
}

func (j *Job) Start() {
	j.p.Start(j.ctx)
}

func (j *Job) Stop() {
	j.cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	done := make(chan struct{})
	go func() {
		j.p.Stop()
		close(done)
	}()
	select {
	case <-done:
		// 停止成功
	case <-stopCtx.Done():
		// 超时处理
	}
}

func (j *Job) Status() string {
	return j.status
}
