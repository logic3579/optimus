package database

import "time"

type Summary struct {
	ID                 uint64            `json:"id"`
	CloudAccountID     uint64            `json:"cloud_account_id"`
	CloudAccountName   string            `json:"cloud_account_name,omitempty"`
	Region             string            `json:"region"`
	DBInstanceID       string            `json:"db_instance_id"`
	Engine             string            `json:"engine"`
	EngineVersion      string            `json:"engine_version"`
	InstanceClass      string            `json:"instance_class"`
	Status             string            `json:"status"`
	Endpoint           string            `json:"endpoint"`
	Port               *int32            `json:"port,omitempty"`
	MultiAZ            bool              `json:"multi_az"`
	PubliclyAccessible bool              `json:"publicly_accessible"`
	StorageGB          *int32            `json:"storage_gb,omitempty"`
	Tags               map[string]string `json:"tags"`
	LastSeenAt         time.Time         `json:"last_seen_at"`
	Deleted            bool              `json:"deleted"`
}

type ListQuery struct {
	AccountID      uint64 `form:"account_id"`
	Region         string `form:"region"`
	Engine         string `form:"engine"`
	Status         string `form:"status"`
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

type ListFilter struct {
	AccountID      uint64
	Region         string
	Engine         string
	Status         string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}
