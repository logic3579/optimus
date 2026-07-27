package release

import (
	"context"
	"strings"

	apperr "optimus-be/internal/infra/errors"
)

// MutationAction is the closed set of P3 Helm release mutations governed by
// delivery. It is intentionally independent of HTTP request fields.
type MutationAction string

const (
	MutationActionInstall   MutationAction = "install"
	MutationActionUpgrade   MutationAction = "upgrade"
	MutationActionRollback  MutationAction = "rollback"
	MutationActionUninstall MutationAction = "uninstall"
)

const maxDeliveryOperationIDLength = 64

// Governance decides whether one P3 release mutation may proceed. A provider
// is expected to permit ordinary unbound applications, deny managed direct
// mutations other than rollback, and validate delivery upgrade capabilities.
type Governance interface {
	AuthorizeMutation(ctx context.Context, applicationID uint64, action MutationAction) error
}

// unboundGovernance preserves the pre-P6 behavior until a delivery provider is
// wired at composition time.
type unboundGovernance struct{}

func (unboundGovernance) AuthorizeMutation(context.Context, uint64, MutationAction) error {
	return nil
}

// deliveryMutationCapability is deliberately private and carried by a private
// typed context key. HTTP payloads cannot construct or replay this value.
type deliveryMutationCapability struct {
	applicationID uint64
	operationID   string
	action        MutationAction
}

type deliveryMutationCapabilityKey struct{}

// withDeliveryUpgrade adds the narrow in-process capability used by the P6
// worker. The only capability this package can issue is an upgrade capability;
// install, uninstall, and rollback cannot be elevated through this helper.
// The issuer stays package-private; UpgradeForDelivery is implemented in this
// package so no other internal package needs authority to mint it.
func withDeliveryUpgrade(ctx context.Context, applicationID uint64, operationID string) context.Context {
	return context.WithValue(ctx, deliveryMutationCapabilityKey{}, deliveryMutationCapability{
		applicationID: applicationID,
		operationID:   operationID,
		action:        MutationActionUpgrade,
	})
}

// deliveryUpgradeAuthorized reports whether ctx contains the exact private
// capability expected by a governance provider. Both identifiers and the
// closed action must match; malformed or empty operation IDs are denied.
func deliveryUpgradeAuthorized(ctx context.Context, applicationID uint64, operationID string) bool {
	if ctx == nil || applicationID == 0 || !validDeliveryOperationID(operationID) {
		return false
	}
	capability, ok := ctx.Value(deliveryMutationCapabilityKey{}).(deliveryMutationCapability)
	return ok &&
		capability.applicationID == applicationID &&
		capability.operationID == operationID &&
		capability.action == MutationActionUpgrade
}

func validDeliveryOperationID(operationID string) bool {
	return operationID == strings.TrimSpace(operationID) &&
		len(operationID) > 0 && len(operationID) <= maxDeliveryOperationIDLength
}

func (s *Service) authorizeMutation(ctx context.Context, applicationID uint64, action MutationAction) error {
	if err := s.governance.AuthorizeMutation(ctx, applicationID, action); err != nil {
		if _, ok := apperr.AsBiz(err); ok {
			return err
		}
		return apperr.Wrap(
			err,
			apperr.CodeDeliveryApplicationUnavailable,
			"delivery.application.unavailable",
			"delivery governance is unavailable",
		)
	}
	return nil
}
