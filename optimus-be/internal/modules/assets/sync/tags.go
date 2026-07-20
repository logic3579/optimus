package sync

import (
	"encoding/json"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// EC2TagMap converts EC2 tags into a nil-safe string map.
func EC2TagMap(tags []ec2types.Tag) map[string]string {
	out := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			out[*tag.Key] = *tag.Value
		}
	}
	return out
}

func EC2TagName(tags []ec2types.Tag) string { return EC2TagMap(tags)["Name"] }

func EC2TagJSON(tags []ec2types.Tag) []byte { return tagJSON(EC2TagMap(tags)) }

// RDSTagMap converts RDS tags into a nil-safe string map.
func RDSTagMap(tags []rdstypes.Tag) map[string]string {
	out := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			out[*tag.Key] = *tag.Value
		}
	}
	return out
}

func RDSTagName(tags []rdstypes.Tag) string { return RDSTagMap(tags)["Name"] }

func RDSTagJSON(tags []rdstypes.Tag) []byte { return tagJSON(RDSTagMap(tags)) }

func tagJSON(tags map[string]string) []byte {
	encoded, err := json.Marshal(tags)
	if err != nil || len(encoded) == 0 || string(encoded) == "null" {
		return []byte("{}")
	}
	return encoded
}
