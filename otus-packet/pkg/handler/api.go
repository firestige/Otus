package handler

type MessageHandler interface {
	Support(msg interface{}) bool
	HandleMessage(msg interface{}) error
}
