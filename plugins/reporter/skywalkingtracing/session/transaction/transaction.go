package transaction

import "firestige.xyz/otus/plugins/reporter/skywalkingtracing/types"

type TimerName string

const (
	// ​用途​：控制 ​INVITE 请求的重传间隔。
	// ​默认值​：初始值 T1（通常 500ms），每次重传后翻倍（指数退避），最大不超过 T2（通常 4s）。
	// ​触发动作​：重传 INVITE 请求，直到收到临时响应（1xx）或达到最大重传次数（默认 6 次）。
	// ​停止条件​：收到 1xx、2xx 或 6xx 响应。
	TimerA TimerName = "Timer A"
	// 适用对象：INVITE 客户端事务（UAC）
	// ​用途​：限制 ​INVITE 事务的总生存时间（等待最终响应的最长时间）。
	// ​默认值​：64 × T1（通常 32s）。
	// ​触发动作​：超时后终止事务，返回超时错误（如 408 Request Timeout）。
	// 出处：RFC 3261, 17.1.1.2
	TimerB TimerName = "Timer B"
	// 适用对象：INVITE 客户端事务（UAC），但只在有代理（Proxy）参与时才需要
	// 作用：防止 UAC 在建立 early dialog 后长时间没有收到任何最终响应（如 2xx/3xx/4xx/5xx/6xx），主要用于代理场景下的 early dialog 清理
	// 默认值：通常 180 秒（可配置）
	// 触发动作：Timer C 超时后，UAC 终止早期对话（early dialog）
	// 出处：RFC 3261, 13.3.1.1
	TimerC TimerName = "Timer C"
	// ​用途​：在 INVITE 事务的 ​​"Completed"​​ 状态中，延迟发送 ACK 到非 2xx 响应（如 3xx-6xx）。
	// ​默认值​：0s（立即发送），但在实际实现中可能短暂延迟以处理网络抖动。
	// ​注意​：ACK 到 2xx 响应是端到端的，不依赖此定时器。
	TimerD TimerName = "Timer D"
	// ​用途​：控制 ​非 INVITE 请求​（如 BYE、OPTIONS）的重传间隔。
	// ​默认值​：同 Timer A（T1 初始值，指数退避）。
	// ​停止条件​：收到最终响应（2xx-6xx）。
	TimerE TimerName = "Timer E"
	// ​用途​：限制 ​非 INVITE 事务的总生存时间。
	// ​默认值​：64 × T1（通常 32s）。
	// ​触发动作​：超时后终止事务。
	TimerF TimerName = "Timer F"
	// ​用途​：在 INVITE 事务的 ​​"Proceeding"​​ 状态中，重传临时响应（1xx）。
	// ​默认值​：T1（通常 500ms），每次重传间隔固定（不指数退避）。
	// ​停止条件​：收到匹配的 ACK 或事务终止。
	TimerG TimerName = "Timer G"
	// ​用途​：在 INVITE 事务的 ​​"Completed"​​ 状态中，等待 ACK 到达的最长时间（仅针对非 2xx 响应）。
	// ​默认值​：64 × T1（通常 32s）。
	// ​触发动作​：超时后终止事务。
	TimerH TimerName = "Timer H"
	// 出处：RFC 3261, Section 17.2.1
	// 作用：Timer I 主要用于 INVITE 服务端事务（UAS），在进入 Confirmed 状态后，等待一段时间（Timer I）再进入 Terminated 状态，用于确保 ACK 的传递和事务清理。
	// 典型值：默认为 T4（通常5秒）。
	// 引用：RFC 3261, 17.2.1
	TimerI TimerName = "Timer I"
	// ​用途​：在 INVITE 事务的 ​​"Completed"​​ 状态中，延迟销毁事务状态（防止重复 ACK）。
	// ​默认值​：64 × T1（通常 32s）。
	// ​触发动作​：超时后进入 ​​"Terminated"​​ 状态。
	TimerJ TimerName = "Timer J"
	// ​用途​：控制 ACK 到非 2xx 响应的重传间隔。
	// ​默认值​：T1（通常 500ms），指数退避到 T2（通常 4s）。
	// ​停止条件​：达到最大重传次数（默认 10 次）。
	TimerK TimerName = "Timer K"
)

type TimerSpan int64

const (
	T1 TimerSpan = 500  // 毫秒，初始重传间隔
	T2 TimerSpan = 4000 // 毫秒，最大重传间隔
	T4 TimerSpan = 5000 // 毫秒，消息传输最大延迟
)

// TransactionEvent 定义所有可能的事件
type TransactionEvent int

const (
	EventSendRequest    TransactionEvent = iota // 发送请求（UAC侧初始动作）
	EventReceiveRequest                         // 收到请求（UAS侧初始动作）
	EventSend1xx                                // 发送1xx临时响应（UAS侧）
	EventSendFinal                              // 发送最终响应（2xx-6xx，UAS侧）
	EventReceive1xx                             // 收到1xx临时响应（UAC侧）
	EventReceive2xx                             // 收到2xx响应（UAC侧）
	EventReceiveFinal                           // 收到最终响应（2xx-6xx，UAC侧）
	EventTimerJ                                 // 非INVITE事务UAS侧，发送最终响应后启动的定时器（等待重传请求，防止丢包，RFC 3261 17.2.2）
	EventTimerK                                 // 非INVITE事务UAC侧，收到最终响应后启动的定时器（清理事务，RFC 3261 17.1.2.2）
	EventSendACK                                // 发送ACK（INVITE事务UAC侧，3xx-6xx响应后）
	EventReceiveACK                             // 收到ACK（INVITE事务UAS侧，3xx-6xx响应后）
	EventTimerG                                 // INVITE事务UAS侧，重传最终响应的定时器（RFC 3261 17.2.1）
	EventTimerH                                 // INVITE事务UAS侧，等待ACK的超时定时器（RFC 3261 17.2.1）
	EventTimerI                                 // INVITE事务UAS侧，收到ACK后等待终结的定时器（RFC 3261 17.2.1）
)

type Transaction interface {
	types.Transaction
	StartTimer(TimerName, TimerSpan)
	CancelTimer(TimerName)
}
