package instance

import (
	"net"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

func TestToSummaryMapsAllFieldsAndKeepsTagsNonNil(t *testing.T) {
	privateIP := net.ParseIP("10.2.3.4")
	publicIP := net.ParseIP("2001:db8::5")
	launchTime := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	lastSeen := launchTime.Add(time.Hour)
	result := toSummary(models.AWSInstance{
		ID: 1, CloudAccountID: 2, Region: "us-east-1", InstanceID: "i-a",
		Name: "web", InstanceType: "t3.small", State: "running",
		PrivateIP: &privateIP, PublicIP: &publicIP, VPCID: "vpc-a", SubnetID: "subnet-a",
		AvailabilityZone: "us-east-1a", LaunchTime: &launchTime,
		Tags: datatypes.JSON(`{"env":"prod"}`), LastSeenAt: lastSeen,
		DeletedAt: gorm.DeletedAt{Time: lastSeen, Valid: true},
	}, "prod-account")

	if result.ID != 1 || result.CloudAccountID != 2 || result.CloudAccountName != "prod-account" {
		t.Fatalf("identity fields = %#v", result)
	}
	if result.PrivateIP != "10.2.3.4" || result.PublicIP != "2001:db8::5" {
		t.Fatalf("IP fields = %#v", result)
	}
	if result.Tags["env"] != "prod" || !result.Deleted || result.LaunchTime == nil || !result.LaunchTime.Equal(launchTime) {
		t.Fatalf("mapped result = %#v", result)
	}

	empty := toSummary(models.AWSInstance{Tags: datatypes.JSON(`null`)}, "")
	if empty.Tags == nil || len(empty.Tags) != 0 {
		t.Fatalf("empty tags = %#v, want non-nil empty map", empty.Tags)
	}
}
