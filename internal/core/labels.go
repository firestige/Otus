// Package core defines core types.
package core

// Labels represents key-value metadata attached by parsers and processors.
type Labels map[string]string

// Label naming constants following {protocol}.{field} convention.
const (
	LabelSIPMethod     = "sip.method"
	LabelSIPCallID     = "sip.call_id"
	LabelSIPFromURI    = "sip.from_uri"
	LabelSIPToURI      = "sip.to_uri"
	LabelSIPStatusCode = "sip.status_code"
	// More labels will be added as protocols are implemented
)
