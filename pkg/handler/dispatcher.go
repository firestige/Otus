package handler

type dispatcherHandler struct {
}

func (d *dispatcherHandler) Support(msg interface{}) bool {
	return true
}

func (d *dispatcherHandler) HandleMessage(msg interface{}) error {
	return nil
}
