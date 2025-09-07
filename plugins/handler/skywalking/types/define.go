package types

type Direction string

const (
	DirectionUnknown  Direction = "unknown"
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type Connection struct {
	SrcIp     string
	SrcPort   int
	DstIp     string
	DstPort   int
	Protocol  string
	Direction Direction
}

type WithConnection interface {
	Connection() *Connection
}

type SessionListener interface {
	OnRequest(req SipRequest, ua UAType)
}

type SipEvents string

var (
	OnRequest               SipEvents = "request"
	OnResponse              SipEvents = "response"
	OnDialogCreated         SipEvents = "dialog_created"
	OnDialogStateChanged    SipEvents = "dialog_state_changed"
	OnDialogTerminated      SipEvents = "dialog_terminated"
	OnTransactionCreated    SipEvents = "transaction_created"
	OnTransactionTerminated SipEvents = "transaction_terminated"
)

type WithValue interface {
	WithValue(key string, value interface{}) WithValue
	Value(key string) interface{}
	ValueAsString(key string) string
	ValueAsInt(key string) int
	ValueAsInt64(key string) int64
	ValueAsBool(key string) bool
	Values() map[string]interface{}
}
