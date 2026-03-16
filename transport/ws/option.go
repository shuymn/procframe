package ws

import "github.com/shuymn/procframe"

// Option configures WebSocket transport behavior.
type Option func(*options)

type options struct {
	errorMapper  procframe.ErrorMapper
	maxInflight  int
	interceptors []procframe.Interceptor
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

// WithInterceptors sets the interceptor chain applied to handler execution.
func WithInterceptors(interceptors ...procframe.Interceptor) Option {
	return func(o *options) {
		o.interceptors = append([]procframe.Interceptor(nil), interceptors...)
	}
}

// WithMaxInflight sets the maximum number of concurrently executing
// handlers per connection. Requests exceeding this limit are rejected
// with CodeUnavailable + retryable=true.
func WithMaxInflight(n int) Option {
	return func(o *options) { o.maxInflight = n }
}
