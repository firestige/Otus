package types

type SipObject interface {
	// returns the whole object as a string in RFC 3261 format
	// this is useful for debugging and logging
	String() string
	Direction() Direction
	CreatedAt() int64
}

type SipContent interface {
	Body() string
	BodyAsBytes() []byte
}

type SipMessage interface {
	SipObject
	Headers() map[string]string
	CallID() string
	CSeq() string
	From() string
	To() string
	StartLine() string
	ViaBranch() string
	IsRequest() bool
}

type SipRequest interface {
	Method() Method
	MethodAsString() string
	RequestLine() string
	SipMessage
	SipContent
}

type SipResponse interface {
	Status() int
	StatusLine() string
	SipMessage
	SipContent
}

type Headers interface {
	Header(name HeaderName) string
	HeaderValues(name HeaderName) []string
	HasHeader(name HeaderName) bool
}

type Method string

const (
	MethodUnknown   Method = "UNKNOWN" // 未知方法,一般是遇到了定义之外的SIP方法
	MethodInvite    Method = "INVITE"
	MethodAck       Method = "ACK"
	MethodOptions   Method = "OPTIONS"
	MethodBye       Method = "BYE"
	MethodCancel    Method = "CANCEL"
	MethodRegister  Method = "REGISTER"
	MethodPrack     Method = "PRACK"     // RFC 3262
	MethodSubscribe Method = "SUBSCRIBE" // RFC 3265
	MethodNotify    Method = "NOTIFY"    // RFC 3265
	MethodUpdate    Method = "UPDATE"    // RFC 3311
	MethodRefer     Method = "REFER"     // RFC 3515
	MethodMessage   Method = "MESSAGE"   // RFC 3428
	MethodInfo      Method = "INFO"      // RFC 2976
	MethodPublish   Method = "PUBLISH"   // RFC 3903
)

type StatusCode int

const (
	// 1xx: Provisional
	Trying                StatusCode = 100
	Ringing               StatusCode = 180
	CallIsBeingForwarded  StatusCode = 181
	Queued                StatusCode = 182
	SessionProgress       StatusCode = 183
	EarlyDialogTerminated StatusCode = 199 // RFC 6228

	// 2xx: Success
	OK       StatusCode = 200
	Accepted StatusCode = 202

	// 3xx: Redirection
	MultipleChoices    StatusCode = 300
	MovedPermanently   StatusCode = 301
	MovedTemporarily   StatusCode = 302
	UseProxy           StatusCode = 305
	AlternativeService StatusCode = 380

	// 4xx: Client Failure
	BadRequest                       StatusCode = 400
	Unauthorized                     StatusCode = 401
	PaymentRequired                  StatusCode = 402
	Forbidden                        StatusCode = 403
	NotFound                         StatusCode = 404
	MethodNotAllowed                 StatusCode = 405
	NotAcceptable                    StatusCode = 406
	ProxyAuthenticationRequired      StatusCode = 407
	RequestTimeout                   StatusCode = 408
	Conflict                         StatusCode = 409
	Gone                             StatusCode = 410
	LengthRequired                   StatusCode = 411
	ConditionalRequestFailed         StatusCode = 412
	RequestEntityTooLarge            StatusCode = 413
	RequestURITooLong                StatusCode = 414
	UnsupportedMediaType             StatusCode = 415
	UnsupportedURI                   StatusCode = 416
	BadExtension                     StatusCode = 420
	ExtensionRequired                StatusCode = 421
	IntervalTooBrief                 StatusCode = 423
	TemporarilyUnavailable           StatusCode = 480
	CallLegOrTransactionDoesNotExist StatusCode = 481
	LoopDetected                     StatusCode = 482
	TooManyHops                      StatusCode = 483
	AddressIncomplete                StatusCode = 484
	Ambiguous                        StatusCode = 485
	BusyHere                         StatusCode = 486
	RequestTerminated                StatusCode = 487
	NotAcceptableHere                StatusCode = 488
	BadEvent                         StatusCode = 489
	RequestPending                   StatusCode = 491
	Undecipherable                   StatusCode = 493

	// 5xx: Server Failure
	ServerInternalError StatusCode = 500
	NotImplemented      StatusCode = 501
	BadGateway          StatusCode = 502
	ServiceUnavailable  StatusCode = 503
	ServerTimeout       StatusCode = 504
	VersionNotSupported StatusCode = 505
	MessageTooLarge     StatusCode = 513
	PreconditionFailure StatusCode = 580

	// 6xx: Global Failure
	BusyEverywhere       StatusCode = 600
	Decline              StatusCode = 603
	DoesNotExistAnywhere StatusCode = 604
	NotAcceptableGlobal  StatusCode = 606
)
