package skills

// InvokeError is a structured error that should be returned to the caller using
// the SSOT invoke error envelope (HTTP 200 + ok=false).
//
// It is intentionally minimal for v0.
type InvokeError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *InvokeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}
