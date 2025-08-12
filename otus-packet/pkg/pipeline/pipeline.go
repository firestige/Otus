package pipeline

type pipeline struct {
	nc *capture.netCapture
}

func (p *pipeline) Start() error {
	return nil
}

func (p *pipeline) Stop() error {
}
