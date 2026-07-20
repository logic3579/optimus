package prometheus

import (
	"context"
	"errors"
	"net"

	apperr "optimus-be/internal/infra/errors"
)

const (
	upstreamUnreachableKey = "observability.query.upstream_unreachable"
	upstreamTimeoutKey     = "observability.query.upstream_timeout"
	upstreamRejectedKey    = "observability.query.upstream_rejected"
	invalidResponseKey     = "observability.query.invalid_response"
	invalidRequestKey      = "observability.query.invalid_request"
)

func mapClientError(err error) error {
	if err == nil {
		return nil
	}
	if be, ok := apperr.AsBiz(err); ok && be.Code == apperr.CodeObservabilityQueryDestinationDenied {
		return apperr.Wrap(err, be.Code, be.MessageKey, be.Message)
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		return apperr.Wrap(err, apperr.CodeObservabilityQueryUpstreamTimeout, upstreamTimeoutKey, "observability query upstream timeout")
	}
	return apperr.Wrap(err, apperr.CodeObservabilityQueryUpstreamUnreachable, upstreamUnreachableKey, "observability query upstream unreachable")
}

func isTimeout(err error) bool { var ne net.Error; return errors.As(err, &ne) && ne.Timeout() }

func rejected(err error) error {
	return apperr.Wrap(err, apperr.CodeObservabilityQueryUpstreamRejected, upstreamRejectedKey, "observability query upstream rejected")
}
func invalidResponse(err error) error {
	return apperr.Wrap(err, apperr.CodeObservabilityQueryInvalidResponse, invalidResponseKey, "observability query invalid response")
}
func invalidRequest(err error) error {
	return apperr.Wrap(err, apperr.CodeObservabilityQueryInvalidRequest, invalidRequestKey, "observability query invalid request")
}
