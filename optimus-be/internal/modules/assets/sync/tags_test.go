package sync

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

func TestEC2TagHelpers(t *testing.T) {
	tags := []ec2types.Tag{
		{Key: aws.String("Name"), Value: aws.String("web-1")},
		{Key: aws.String("env"), Value: aws.String("prod")},
		{Key: nil, Value: aws.String("ignored")},
		{Key: aws.String("ignored"), Value: nil},
	}

	got := EC2TagMap(tags)
	if len(got) != 2 || got["Name"] != "web-1" || got["env"] != "prod" {
		t.Fatalf("EC2TagMap() = %#v", got)
	}
	if got := EC2TagName(tags); got != "web-1" {
		t.Fatalf("EC2TagName() = %q", got)
	}
	assertTagJSON(t, EC2TagJSON(tags), `{"Name":"web-1","env":"prod"}`)
	assertTagJSON(t, EC2TagJSON(nil), `{}`)
}

func TestRDSTagHelpers(t *testing.T) {
	tags := []rdstypes.Tag{
		{Key: aws.String("team"), Value: aws.String("platform")},
		{Key: aws.String("Name"), Value: aws.String("primary")},
		{Key: nil, Value: aws.String("ignored")},
		{Key: aws.String("ignored"), Value: nil},
	}

	got := RDSTagMap(tags)
	if len(got) != 2 || got["Name"] != "primary" || got["team"] != "platform" {
		t.Fatalf("RDSTagMap() = %#v", got)
	}
	if got := RDSTagName(tags); got != "primary" {
		t.Fatalf("RDSTagName() = %q", got)
	}
	assertTagJSON(t, RDSTagJSON(tags), `{"Name":"primary","team":"platform"}`)
	assertTagJSON(t, RDSTagJSON(nil), `{}`)
}

func assertTagJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	if got == nil {
		t.Fatal("tag JSON must be non-nil")
	}
	if string(got) != want {
		t.Fatalf("tag JSON = %q, want %q", got, want)
	}
	var decoded map[string]string
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("tag JSON is invalid: %v", err)
	}
}
