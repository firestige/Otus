package types

type Transaction interface {
	ID() string
	Type() TransactionType
	UA() UAType
	Request() SipRequest
	LastResponse() SipResponse
	CreatedAt() int64
	UpdatedAt() int64
	IsTerminated() bool // 新增
	Error() error
}

type TransactionListener interface {
	OnTransactionCreated(tx Transaction)
	OnTransactionStateChanged(tx Transaction)
	OnTransactionTerminated(tx Transaction)
	OnTransactionTimeout(tx Transaction)
	OnTransactionError(tx Transaction, err error)
}

type TransactionType int

const (
	InviteTransactionType TransactionType = iota
	NonInviteTransactionType
)

type InviteTransactionState int

const (
	InviteTransactionStateCalling InviteTransactionState = iota
	InviteTransactionStateProceeding
	InviteTransactionStateCompleted
	InviteTransactionStateConfirmed
	InviteTransactionStateTerminated
)

type NonInviteTransactionState int

const (
	NonInviteTransactionStateTrying NonInviteTransactionState = iota
	NonInviteTransactionStateProceeding
	NonInviteTransactionStateCompleted
	NonInviteTransactionStateTerminated
)

type UAType int

const (
	UAUnknown UAType = iota
	UAClient
	UAServer
)

type TransactionState interface {
	IsTerminal() bool
	String() string
}

// InviteTransactionState 实现 TransactionState
func (s InviteTransactionState) IsTerminal() bool {
	return s == InviteTransactionStateTerminated
}
func (s InviteTransactionState) String() string {
	switch s {
	case InviteTransactionStateCalling:
		return "Calling"
	case InviteTransactionStateProceeding:
		return "Proceeding"
	case InviteTransactionStateCompleted:
		return "Completed"
	case InviteTransactionStateConfirmed:
		return "Confirmed"
	case InviteTransactionStateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

// NonInviteTransactionState 实现 TransactionState
func (s NonInviteTransactionState) IsTerminal() bool {
	return s == NonInviteTransactionStateTerminated
}
func (s NonInviteTransactionState) String() string {
	switch s {
	case NonInviteTransactionStateTrying:
		return "Trying"
	case NonInviteTransactionStateProceeding:
		return "Proceeding"
	case NonInviteTransactionStateCompleted:
		return "Completed"
	case NonInviteTransactionStateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}
