package procframe

// Response wraps a typed message with transport-independent metadata.
type Response[T any] struct {
	Msg  *T
	Meta *Meta
}
