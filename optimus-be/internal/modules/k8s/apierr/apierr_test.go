package apierr_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/k8s/apierr"
)

func TestMapAPIError_Nil(t *testing.T) {
	require.Nil(t, apierr.MapAPIError(nil))
}

func TestMapAPIError_NotFound(t *testing.T) {
	err := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "x")
	got := apierr.MapAPIError(err)
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeNotFound, be.Code)
	require.Equal(t, "k8s.apiserver.not_found", be.MessageKey)
}

func TestMapAPIError_Forbidden(t *testing.T) {
	err := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "x", errors.New("nope"))
	got := apierr.MapAPIError(err)
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeAPIServerForbidden, be.Code)
	require.Equal(t, "k8s.apiserver.forbidden", be.MessageKey)
}

func TestMapAPIError_Unauthorized(t *testing.T) {
	err := apierrors.NewUnauthorized("bad creds")
	got := apierr.MapAPIError(err)
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeAPIServerUnauthorized, be.Code)
	require.Equal(t, "k8s.apiserver.unauthorized", be.MessageKey)
}

func TestMapAPIError_ServerTimeout(t *testing.T) {
	err := apierrors.NewServerTimeout(schema.GroupResource{Resource: "pods"}, "list", 1)
	got := apierr.MapAPIError(err)
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeClusterUnreachable, be.Code)
	require.Equal(t, "k8s.cluster.unreachable", be.MessageKey)
}

func TestMapAPIError_NetworkConnRefused(t *testing.T) {
	got := apierr.MapAPIError(errors.New("dial tcp 127.0.0.1:6443: connection refused"))
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeClusterUnreachable, be.Code)
	require.Equal(t, "k8s.cluster.unreachable", be.MessageKey)
}

func TestMapAPIError_NetworkNoSuchHost(t *testing.T) {
	got := apierr.MapAPIError(errors.New("dial tcp: lookup nowhere.invalid: no such host"))
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeClusterUnreachable, be.Code)
}

func TestMapAPIError_NetworkIOTimeout(t *testing.T) {
	got := apierr.MapAPIError(errors.New("dial tcp 10.0.0.1:6443: i/o timeout"))
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeClusterUnreachable, be.Code)
}

func TestMapAPIError_Other(t *testing.T) {
	got := apierr.MapAPIError(errors.New("anything else"))
	be, ok := got.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", got)
	require.Equal(t, apperr.CodeAPIServerOther, be.Code)
	require.Equal(t, "k8s.apiserver.other", be.MessageKey)
}
