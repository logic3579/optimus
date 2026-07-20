package errs

import infraerrors "optimus-be/internal/infra/errors"

const (
	CodeAssetsCloudAccountInUse        = infraerrors.CodeAssetsCloudAccountInUse
	CodeAssetsCloudAccountNotFound     = infraerrors.CodeAssetsCloudAccountNotFound
	CodeAssetsCloudAccountNameConflict = infraerrors.CodeAssetsCloudAccountNameConflict
	CodeAssetsRegionInvalid            = infraerrors.CodeAssetsRegionInvalid
	CodeAssetsProviderUnsupported      = infraerrors.CodeAssetsProviderUnsupported
	CodeAssetsCloudAccountDisabled     = infraerrors.CodeAssetsCloudAccountDisabled
	CodeAssetsCloudKeyNotFound         = infraerrors.CodeAssetsCloudKeyNotFound
	CodeAssetsVPCNotFound              = infraerrors.CodeAssetsVPCNotFound
	CodeAssetsSyncBusy                 = infraerrors.CodeAssetsSyncBusy
	CodeAssetsAWSUnauthorized          = infraerrors.CodeAssetsAWSUnauthorized
	CodeAssetsAWSForbidden             = infraerrors.CodeAssetsAWSForbidden
	CodeAssetsAWSUnreachable           = infraerrors.CodeAssetsAWSUnreachable
	CodeAssetsAWSThrottled             = infraerrors.CodeAssetsAWSThrottled
	CodeAssetsAWSOther                 = infraerrors.CodeAssetsAWSOther
	CodeAssetsAWSConfig                = infraerrors.CodeAssetsAWSConfig
)

const (
	KeyCloudAccountInUse        = "assets.cloud_account.in_use"
	KeyCloudAccountNotFound     = "assets.cloud_account.not_found"
	KeyCloudAccountNameConflict = "assets.cloud_account.name_conflict"
	KeyRegionInvalid            = "assets.region.invalid"
	KeyProviderUnsupported      = "assets.provider.unsupported"
	KeyCloudAccountDisabled     = "assets.cloud_account.disabled"
	KeyCloudKeyNotFound         = "assets.cloudkey.not_found"
	KeyVPCNotFound              = "assets.vpc.not_found"
	KeySyncBusy                 = "assets.sync.busy"
	KeyAWSUnauthorized          = "assets.aws.unauthorized"
	KeyAWSForbidden             = "assets.aws.forbidden"
	KeyAWSUnreachable           = "assets.aws.unreachable"
	KeyAWSThrottled             = "assets.aws.throttled"
	KeyAWSOther                 = "assets.aws.other"
	KeyAWSConfig                = "assets.aws.config"
)
