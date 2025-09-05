package utils

import (
	"fmt"
	"strings"

	"firestige.xyz/otus/plugins/filter/skywalking/types"
)

func BuildTransactionID(msg types.SipMessage) string {
	// 使用 Call-ID 、 Cseq 和 Via头的branch 作为事务 ID 的基础(为了避免空格引起的问题，使用下划线替代CSeq中的空格)
	callID := msg.CallID()
	cseq := strings.ReplaceAll(msg.CSeq(), " ", "_")
	branch := msg.ViaBranch()
	return fmt.Sprintf("%s|%s|%s", callID, cseq, branch)
}

func CreateTransactionType(req types.SipRequest) types.TransactionType {
	switch req.Method() {
	case types.MethodInvite, types.MethodAck, types.MethodInfo, types.MethodBye, types.MethodCancel:
		return types.InviteTransactionType
	default:
		return types.NonInviteTransactionType
	}
}

// buildDialogID constructs a dialog ID based on the Call-ID, From-Tag, and To-Tag.
// If To-Tag is empty, it returns a boolean indicating whether the dialog ID is known.
// If To-Tag is not empty, it returns false, indicating that the dialog ID is not known.
// args:
//   - msg: SipMessage containing Call-ID, From-Tag, and To-Tag
//   - earlyDialog: true-early dialog, false-regular dialog
//
// returns:
//   - string: constructed dialog ID
func BuildDialogID(msg types.SipMessage) string {
	// 使用 Call-ID 和 From-Tag 作为事务 ID 的基础
	callID := msg.CallID()
	_, fromTag := ExtractURIAndTag(msg.From())
	return fmt.Sprintf("%s|%s", callID, fromTag)
}
