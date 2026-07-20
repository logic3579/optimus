package query

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
	Step         time.Duration `json:"step" swaggertype:"string" example:"1m"`
	EnrichAssets bool          `json:"enrich_assets"`
	Queries      []Query       `json:"queries"`
}

func (r *RangeRequest) UnmarshalJSON(data []byte) error {
	type wire struct {
		DatasourceID uint64          `json:"datasource_id"`
		Start        time.Time       `json:"start"`
		End          time.Time       `json:"end"`
		Step         json.RawMessage `json:"step"`
		EnrichAssets bool            `json:"enrich_assets"`
		Queries      []Query         `json:"queries"`
	}
	var v wire
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&v); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	var raw string
	if len(v.Step) == 0 || json.Unmarshal(v.Step, &raw) != nil || raw == "" {
		return errors.New("step must be a duration string")
	}
	step, err := time.ParseDuration(raw)
	if err != nil {
		return errors.New("invalid step duration")
	}
	*r = RangeRequest{DatasourceID: v.DatasourceID, Start: v.Start, End: v.End, Step: step, EnrichAssets: v.EnrichAssets, Queries: v.Queries}
	return nil
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
