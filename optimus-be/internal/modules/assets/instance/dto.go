package instance

import "time"

type Summary struct {
	ID               uint64            `json:"id"`
	CloudAccountID   uint64            `json:"cloud_account_id"`
	CloudAccountName string            `json:"cloud_account_name,omitempty"`
	Region           string            `json:"region"`
	InstanceID       string            `json:"instance_id"`
	Name             string            `json:"name"`
	InstanceType     string            `json:"instance_type"`
	State            string            `json:"state"`
	PrivateIP        string            `json:"private_ip,omitempty"`
	PublicIP         string            `json:"public_ip,omitempty"`
	VPCID            string            `json:"vpc_id,omitempty"`
	SubnetID         string            `json:"subnet_id,omitempty"`
	AvailabilityZone string            `json:"availability_zone,omitempty"`
	LaunchTime       *time.Time        `json:"launch_time,omitempty"`
	Tags             map[string]string `json:"tags"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	Deleted          bool              `json:"deleted"`
}

type ListQuery struct {
	AccountID      uint64 `form:"account_id"`
	Region         string `form:"region"`
	State          string `form:"state"`
	VPCID          string `form:"vpc_id"`
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1"  binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

// ListFilter is validated before it reaches persistence.
type ListFilter struct {
	AccountID      uint64
	Region         string
	State          string
	VPCID          string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
	Offset         int
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}
