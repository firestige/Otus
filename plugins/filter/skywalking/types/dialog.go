package types

type Dialog interface {
	ID() string
	UA() UAType
	CallID() string
	LocalTag() string
	RemoteTag() string
	LocalURI() string
	RemoteURI() string
	CreatedAt() int64
	UpdatedAt() int64
	Metadatas() map[string]string
}

type DialogState int

const (
	DialogStateEarly DialogState = iota
	DialogStateConfirmed
	DialogStateTerminated
)

type DialogListener interface {
	OnDialogCreated(dialog Dialog)
	OnDialogStateChanged(dialog Dialog)
	OnDialogTerminated(dialog Dialog)
}
