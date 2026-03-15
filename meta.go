package procframe

// Meta carries transport-independent metadata for a procedure call.
type Meta struct {
	Procedure string
	RequestID string
	SessionID string
	Labels    map[string]string
}
