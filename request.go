package procframe

// Request wraps a typed message with transport-independent metadata.
type Request[T any] struct {
	Msg  *T
	Meta Meta
}
