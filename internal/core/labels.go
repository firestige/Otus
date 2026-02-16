// Package core defines core types.
package core

// Labels represents key-value metadata.
type Labels map[string]string

// Label naming constants.
const (
LabelSIPMethod     = "sip.method"
LabelSIPCallID     = "sip.call_id"
LabelSIPFromURI    = "sip.from_uri"
LabelSIPToURI      = "sip.to_uri"
LabelSIPStatusCode = "sip.status_code"
)
