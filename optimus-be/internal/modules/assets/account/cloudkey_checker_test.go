package account_test

import (
	"optimus-be/internal/modules/assets/account"
	"optimus-be/internal/modules/credentials/cloudkey"
)

var _ account.CloudKeyExistenceChecker = (*cloudkey.Service)(nil)
