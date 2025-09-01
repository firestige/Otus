package dialog

import "firestige.xyz/otus/plugins/reporter/skywalkingtracing/types"

// DialogEvent 定义所有可能的事件
type DialogEvent int

const (
	EventSendRequest                DialogEvent = iota // 发送请求（UAC侧初始动作）
	EventReceiveRequest                                // 接收请求（UAS侧初始动作）
	EventSendProvisionalResponse                       // 发送临时响应（1xx）（UAS侧）
	EventReceiveProvisionalResponse                    // 接收临时响应（1xx）（UAC侧）
	EventSend2xxResponse                               // 发送2xx响应
	EventReceive2xxResponse                            // 接收2xx响应
	EventSendNon2xxFinalResponse                       // 发送非2xx最终响应（3xx/4xx/5xx/6xx）
	EventReceiveNon2xxFinalResponse                    // 接收非2xx最终响应（3xx/4xx/5xx/6xx）
	EventSendBYERequest                                // 发送BYE请求
	EventReceiveBYERequest                             // 接收BYE请求
	EventTerminate                                     // 强制终止
)

type Dialog interface {
	types.Dialog
}
