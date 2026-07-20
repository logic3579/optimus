package query

import (
	"time"

	"optimus-be/internal/modules/observability/prometheus"
)

type Query struct {
	RefID  string `json:"ref_id"`
	PromQL string `json:"promql"`
}
type InstantRequest struct {
	DatasourceID uint64    `json:"datasource_id"`
	Time         time.Time `json:"time"`
	EnrichAssets bool      `json:"enrich_assets"`
	Queries      []Query   `json:"queries"`
}
type RangeRequest struct {
	DatasourceID uint64        `json:"datasource_id"`
	Start        time.Time     `json:"start"`
	End          time.Time     `json:"end"`
	Step         time.Duration `json:"step"`
	EnrichAssets bool          `json:"enrich_assets"`
	Queries      []Query       `json:"queries"`
}
type ItemError struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	MessageKey string `json:"message_key,omitempty"`
}
type ItemResult struct {
	RefID  string             `json:"ref_id"`
	Result *prometheus.Result `json:"result,omitempty"`
	Error  *ItemError         `json:"error,omitempty"`
}
type BatchResult struct {
	Results      []ItemResult            `json:"results"`
	AssetContext map[string]AssetSummary `json:"asset_context,omitempty"`
}
type AssetSummary struct {
	InstanceID   string `json:"instance_id"`
	Name         string `json:"name,omitempty"`
	AccountID    int64  `json:"account_id"`
	AccountName  string `json:"account_name,omitempty"`
	Region       string `json:"region"`
	InstanceType string `json:"instance_type,omitempty"`
	State        string `json:"state,omitempty"`
	PrivateIP    string `json:"private_ip"`
	PublicIP     string `json:"public_ip,omitempty"`
	VPCID        string `json:"vpc_id,omitempty"`
	SubnetID     string `json:"subnet_id,omitempty"`
}
