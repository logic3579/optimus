package release

import "context"

// IssueDeliveryUpgradeForTest lets the external test package verify the
// read-only public verifier without adding a production capability issuer.
func IssueDeliveryUpgradeForTest(ctx context.Context, applicationID uint64, operationID string) context.Context {
	return withDeliveryUpgrade(ctx, applicationID, operationID)
}
