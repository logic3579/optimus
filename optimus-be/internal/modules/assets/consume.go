// Package assets exposes a read-only Consumer seam so downstream modules can
// look up discovered resource snapshots without going through HTTP. Resource
// lookups intentionally have no purpose parameter and produce no audit rows.
package assets

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"strings"

	apperr "optimus-be/internal/infra/errors"

	"gorm.io/gorm"
)

// Instance is the read-side view of a live EC2 instance snapshot.
type Instance struct {
	AccountID    int64
	AccountName  string
	Region       string
	InstanceID   string
	Name         string
	InstanceType string
	State        string
	PrivateIP    netip.Addr
	PublicIP     netip.Addr
	VPCID        string
	SubnetID     string
}

// ErrAssetsInstanceNotFound is returned when a single-instance lookup has no
// live match. Callers should test it with errors.Is.
var ErrAssetsInstanceNotFound = errors.New("assets: instance not found")

// Consumer is the stable read-only assets seam used by downstream modules.
type Consumer interface {
	LookupInstanceByPrivateIP(ctx context.Context, ip netip.Addr) (*Instance, error)
	LookupInstanceByID(ctx context.Context, accountID int64, region, instanceID string) (*Instance, error)
	ListInstancesByVPC(ctx context.Context, accountID int64, region, vpcID string) ([]Instance, error)
}

type consumer struct{ db *gorm.DB }

// NewConsumer constructs a Consumer backed by the assets database tables.
func NewConsumer(db *gorm.DB) Consumer { return &consumer{db: db} }

func (c *consumer) LookupInstanceByPrivateIP(ctx context.Context, ip netip.Addr) (*Instance, error) {
	if !ip.IsValid() {
		return nil, validationError("IP address must be valid")
	}
	canonical := ip.Unmap()
	if canonical.Is4() {
		return c.findOne(ctx,
			"(aws_instances.private_ip = ? OR aws_instances.private_ip = ?)",
			canonical.String(), "::ffff:"+canonical.String(),
		)
	}
	return c.findOne(ctx, "aws_instances.private_ip = ?", canonical.String())
}

func (c *consumer) LookupInstanceByID(ctx context.Context, accountID int64, region, instanceID string) (*Instance, error) {
	region = strings.TrimSpace(region)
	instanceID = strings.TrimSpace(instanceID)
	if err := validateTuple(accountID, region, instanceID, "instance ID"); err != nil {
		return nil, err
	}
	return c.findOne(ctx,
		"aws_instances.cloud_account_id = ? AND aws_instances.region = ? AND aws_instances.instance_id = ?",
		accountID, region, instanceID,
	)
}

func (c *consumer) ListInstancesByVPC(ctx context.Context, accountID int64, region, vpcID string) ([]Instance, error) {
	region = strings.TrimSpace(region)
	vpcID = strings.TrimSpace(vpcID)
	if err := validateTuple(accountID, region, vpcID, "VPC ID"); err != nil {
		return nil, err
	}

	rows := make([]consumerRow, 0)
	err := c.baseQuery(ctx).
		Where("aws_instances.cloud_account_id = ?", accountID).
		Where("aws_instances.region = ?", region).
		Where("aws_instances.vpc_id = ?", vpcID).
		Order("aws_instances.last_seen_at DESC, aws_instances.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, databaseError(err)
	}

	items := make([]Instance, len(rows))
	for i := range rows {
		items[i] = rows[i].instance()
	}
	return items, nil
}

func (c *consumer) findOne(ctx context.Context, where string, args ...any) (*Instance, error) {
	var row consumerRow
	err := c.baseQuery(ctx).
		Where(where, args...).
		Order("aws_instances.last_seen_at DESC, aws_instances.id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, databaseError(err)
	}
	if row.ID == 0 {
		return nil, ErrAssetsInstanceNotFound
	}
	result := row.instance()
	return &result, nil
}

// consumerRow deliberately projects inet columns through host() into strings.
// Scanning PostgreSQL inet directly into models.AWSInstance's *net.IP fields is
// driver-dependent and fails with the pgx/GORM combination used by Optimus.
type consumerRow struct {
	ID           int64          `gorm:"column:id"`
	AccountID    int64          `gorm:"column:cloud_account_id"`
	AccountName  string         `gorm:"column:account_name"`
	Region       string         `gorm:"column:region"`
	InstanceID   string         `gorm:"column:instance_id"`
	Name         string         `gorm:"column:name"`
	InstanceType string         `gorm:"column:instance_type"`
	State        string         `gorm:"column:state"`
	PrivateIP    sql.NullString `gorm:"column:private_ip_text"`
	PublicIP     sql.NullString `gorm:"column:public_ip_text"`
	VPCID        string         `gorm:"column:vpc_id"`
	SubnetID     string         `gorm:"column:subnet_id"`
}

func (c *consumer) baseQuery(ctx context.Context) *gorm.DB {
	return c.db.WithContext(ctx).
		Table("aws_instances").
		Select(`
			aws_instances.id,
			aws_instances.cloud_account_id,
			cloud_accounts.name AS account_name,
			aws_instances.region,
			aws_instances.instance_id,
			aws_instances.name,
			aws_instances.instance_type,
			aws_instances.state,
			host(aws_instances.private_ip) AS private_ip_text,
			host(aws_instances.public_ip) AS public_ip_text,
			aws_instances.vpc_id,
			aws_instances.subnet_id`).
		Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = aws_instances.cloud_account_id").
		Where("aws_instances.deleted_at IS NULL")
}

func (r consumerRow) instance() Instance {
	return Instance{
		AccountID: r.AccountID, AccountName: r.AccountName,
		Region: r.Region, InstanceID: r.InstanceID, Name: r.Name,
		InstanceType: r.InstanceType, State: r.State,
		PrivateIP: parseProjectedIP(r.PrivateIP.String, r.PrivateIP.Valid),
		PublicIP:  parseProjectedIP(r.PublicIP.String, r.PublicIP.Valid),
		VPCID:     r.VPCID, SubnetID: r.SubnetID,
	}
}

func parseProjectedIP(value string, valid bool) netip.Addr {
	if !valid {
		return netip.Addr{}
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func validateTuple(accountID int64, region, resourceID, resourceName string) error {
	if accountID <= 0 {
		return validationError("account ID must be positive")
	}
	if region == "" {
		return validationError("region is required")
	}
	if resourceID == "" {
		return validationError(resourceName + " is required")
	}
	return nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

func databaseError(err error) error {
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}
