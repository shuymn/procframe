package ws

import "github.com/shuymn/procframe"

// Option configures WebSocket transport behavior.
type Option func(*options)

type options struct {
	errorMapper procframe.ErrorMapper
	maxInflight int
}

func defaultOptions() options {
	return options{
		maxInflight: 64,
	}
}

// WithErrorMapper sets the boundary mapper used to classify errors
// that are not already wrapped as [procframe.StatusError].
func WithErrorMapper(mapper procframe.ErrorMapper) Option {
	return func(o *options) { o.errorMapper = mapper }
}

// WithMaxInflight sets the maximum number of concurrently executing
// handlers per connection. Requests exceeding this limit are rejected
// with CodeUnavailable + retryable=true.
func WithMaxInflight(n int) Option {
	return func(o *options) { o.maxInflight = n }
}
