package dialog

import "firestige.xyz/otus/plugins/handler/skywalking/types"

// DialogEvent 定义所有可能的事件
type DialogEvent string

const (
	EventDialogCreated    DialogEvent = "DialogCreated"    // 对话创建
	EventDialogTerminated DialogEvent = "DialogTerminated" // 对话终止
	EventDialogTimeout    DialogEvent = "DialogTimeout"    // 对话超时，RF3261 并没有规定会话超时，但是实际应用中可能出现对方没有发送 BYE 或 CANCEL 的情况，所以需要设置一个超时时间
)

type Dialog interface {
	types.Dialog
}
