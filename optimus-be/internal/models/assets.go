package models

import (
	"net"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type CloudAccount struct {
	ID             uint64      `gorm:"primaryKey"`
	Name           string      `gorm:"size:128;not null"`
	Provider       string      `gorm:"size:16;not null"`
	CloudKeyID     uint64      `gorm:"column:cloudkey_id;not null;index"`
	EnabledRegions StringArray `gorm:"type:text[];not null"`
	Enabled        bool        `gorm:"not null;default:true"`
	Description    string      `gorm:"type:text;not null;default:''"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (CloudAccount) TableName() string { return "cloud_accounts" }

type AWSInstance struct {
	ID               uint64  `gorm:"primaryKey"`
	CloudAccountID   uint64  `gorm:"column:cloud_account_id;not null"`
	Region           string  `gorm:"size:32;not null"`
	InstanceID       string  `gorm:"column:instance_id;size:32;not null"`
	Name             string  `gorm:"type:text;not null;default:''"`
	InstanceType     string  `gorm:"size:32;not null;default:''"`
	State            string  `gorm:"size:16;not null;default:''"`
	PrivateIP        *net.IP `gorm:"type:inet"`
	PublicIP         *net.IP `gorm:"type:inet"`
	VPCID            string  `gorm:"column:vpc_id;size:32;not null;default:''"`
	SubnetID         string  `gorm:"column:subnet_id;size:32;not null;default:''"`
	AvailabilityZone string  `gorm:"size:32;not null;default:''"`
	LaunchTime       *time.Time
	Tags             datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt       time.Time      `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

func (AWSInstance) TableName() string { return "aws_instances" }

type AWSVPC struct {
	ID             uint64         `gorm:"primaryKey"`
	CloudAccountID uint64         `gorm:"column:cloud_account_id;not null"`
	Region         string         `gorm:"size:32;not null"`
	VPCID          string         `gorm:"column:vpc_id;size:32;not null"`
	Name           string         `gorm:"type:text;not null;default:''"`
	CIDRBlock      *string        `gorm:"column:cidr_block;type:cidr"`
	IsDefault      bool           `gorm:"not null;default:false"`
	State          string         `gorm:"size:16;not null;default:''"`
	Tags           datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt     time.Time      `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (AWSVPC) TableName() string { return "aws_vpcs" }

type AWSSubnet struct {
	ID               uint64         `gorm:"primaryKey"`
	CloudAccountID   uint64         `gorm:"column:cloud_account_id;not null"`
	Region           string         `gorm:"size:32;not null"`
	SubnetID         string         `gorm:"column:subnet_id;size:32;not null"`
	VPCID            string         `gorm:"column:vpc_id;size:32;not null"`
	CIDRBlock        *string        `gorm:"column:cidr_block;type:cidr"`
	AvailabilityZone string         `gorm:"size:32;not null;default:''"`
	Name             string         `gorm:"type:text;not null;default:''"`
	Tags             datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt       time.Time      `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

func (AWSSubnet) TableName() string { return "aws_subnets" }

type AWSDatabase struct {
	ID                 uint64 `gorm:"primaryKey"`
	CloudAccountID     uint64 `gorm:"column:cloud_account_id;not null"`
	Region             string `gorm:"size:32;not null"`
	DBInstanceID       string `gorm:"column:db_instance_id;size:64;not null"`
	Engine             string `gorm:"size:32;not null;default:''"`
	EngineVersion      string `gorm:"size:32;not null;default:''"`
	InstanceClass      string `gorm:"size:32;not null;default:''"`
	Status             string `gorm:"size:32;not null;default:''"`
	Endpoint           string `gorm:"type:text;not null;default:''"`
	Port               *int32
	MultiAZ            bool           `gorm:"column:multi_az;not null;default:false"`
	PubliclyAccessible bool           `gorm:"not null;default:false"`
	StorageGB          *int32         `gorm:"column:storage_gb"`
	Tags               datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt         time.Time      `gorm:"not null"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (AWSDatabase) TableName() string { return "aws_databases" }

type AssetsSyncRun struct {
	ID                uint64 `gorm:"primaryKey"`
	CloudAccountID    uint64 `gorm:"column:cloud_account_id;not null"`
	Region            string `gorm:"size:32;not null"`
	ResourceType      string `gorm:"size:16;not null"`
	StartedAt         time.Time
	FinishedAt        *time.Time
	Status            string `gorm:"size:16;not null"`
	ItemsSeen         int32  `gorm:"not null;default:0"`
	ItemsSoftDeleted  int32  `gorm:"column:items_softdeleted;not null;default:0"`
	Error             string `gorm:"type:text;not null;default:''"`
	ErrorCode         int32  `gorm:"not null;default:0"`
	Trigger           string `gorm:"size:16;not null"`
	TriggeredByUserID *uint64
	CreatedAt         time.Time
}

func (AssetsSyncRun) TableName() string { return "assets_sync_runs" }
