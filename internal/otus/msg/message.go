package msg

// TODO 批量消息发送
type BatchData []interface{}

// 消息上下文
type OutputMessage struct {
	Message map[string]interface{}
}
