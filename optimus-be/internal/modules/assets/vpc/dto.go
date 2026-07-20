package vpc

import "time"

type Summary struct {
	ID               uint64            `json:"id"`
	CloudAccountID   uint64            `json:"cloud_account_id"`
	CloudAccountName string            `json:"cloud_account_name,omitempty"`
	Region           string            `json:"region"`
	VPCID            string            `json:"vpc_id"`
	Name             string            `json:"name"`
	CIDRBlock        string            `json:"cidr_block,omitempty"`
	IsDefault        bool              `json:"is_default"`
	State            string            `json:"state"`
	Tags             map[string]string `json:"tags"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	Deleted          bool              `json:"deleted"`
}

type SubnetSummary struct {
	ID               uint64            `json:"id"`
	CloudAccountID   uint64            `json:"cloud_account_id"`
	Region           string            `json:"region"`
	SubnetID         string            `json:"subnet_id"`
	VPCID            string            `json:"vpc_id"`
	Name             string            `json:"name"`
	CIDRBlock        string            `json:"cidr_block,omitempty"`
	AvailabilityZone string            `json:"availability_zone"`
	Tags             map[string]string `json:"tags"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	Deleted          bool              `json:"deleted"`
}

type ListQuery struct {
	AccountID      uint64 `form:"account_id"`
	Region         string `form:"region"`
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

type SubnetListQuery struct {
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

type ListFilter struct {
	AccountID      uint64
	Region         string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
}

type SubnetListFilter struct {
	CloudAccountID uint64
	Region         string
	VPCID          string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}

type SubnetListResponse struct {
	Items []SubnetSummary `json:"items"`
	Total int64           `json:"total"`
}
