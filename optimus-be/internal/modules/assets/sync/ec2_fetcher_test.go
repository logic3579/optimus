package sync

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const describeInstancesPage1 = `<?xml version="1.0" encoding="UTF-8"?>
<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <nextToken>page-2</nextToken>
  <reservationSet>
    <item>
      <reservationId>r-001</reservationId>
      <instancesSet>
        <item>
          <instanceId>i-aaa</instanceId>
          <instanceType>t3.micro</instanceType>
          <instanceState><code>16</code><name>running</name></instanceState>
          <privateIpAddress>10.0.0.5</privateIpAddress>
          <ipAddress>54.10.20.30</ipAddress>
          <vpcId>vpc-1</vpcId>
          <subnetId>subnet-1</subnetId>
          <placement><availabilityZone>us-east-1a</availabilityZone></placement>
          <launchTime>2024-01-02T03:04:05Z</launchTime>
          <tagSet>
            <item><key>Name</key><value>web-1</value></item>
            <item><key>env</key><value>prod</value></item>
          </tagSet>
        </item>
        <item><instanceId>i-bbb</instanceId><instanceType>m7g.large</instanceType></item>
      </instancesSet>
    </item>
  </reservationSet>
</DescribeInstancesResponse>`

const describeInstancesPage2 = `<?xml version="1.0" encoding="UTF-8"?>
<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <reservationSet>
    <item><instancesSet><item><instanceId>i-ccc</instanceId><instanceType>c7g.large</instanceType></item></instancesSet></item>
  </reservationSet>
</DescribeInstancesResponse>`

func TestEC2Fetcher_PaginatesFlattensAndMapsFields(t *testing.T) {
	var requests atomic.Int32
	client := newEC2Client(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		call := requests.Add(1)
		if got := request.Form.Get("Action"); got != "DescribeInstances" {
			t.Errorf("Action = %q", got)
		}
		writer.Header().Set("Content-Type", "text/xml")
		switch call {
		case 1:
			if token := request.Form.Get("NextToken"); token != "" {
				t.Errorf("first NextToken = %q", token)
			}
			_, _ = writer.Write([]byte(describeInstancesPage1))
		case 2:
			if token := request.Form.Get("NextToken"); token != "page-2" {
				t.Errorf("second NextToken = %q", token)
			}
			_, _ = writer.Write([]byte(describeInstancesPage2))
		default:
			t.Errorf("unexpected request %d", call)
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))

	got, err := (EC2Fetcher{}).FetchInstances(context.Background(), &Clients{EC2: client})
	if err != nil {
		t.Fatalf("FetchInstances: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	if len(got) != 3 {
		t.Fatalf("instances = %d, want 3: %#v", len(got), got)
	}
	instance := got[0]
	if instance.InstanceID != "i-aaa" || instance.InstanceType != "t3.micro" || instance.State != "running" || instance.Name != "web-1" {
		t.Errorf("identity fields = %#v", instance)
	}
	if instance.VPCID != "vpc-1" || instance.SubnetID != "subnet-1" || instance.AvailabilityZone != "us-east-1a" {
		t.Errorf("network fields = %#v", instance)
	}
	if instance.PrivateIP == nil || instance.PrivateIP.String() != "10.0.0.5" || instance.PublicIP == nil || instance.PublicIP.String() != "54.10.20.30" {
		t.Errorf("IP fields = private %v public %v", instance.PrivateIP, instance.PublicIP)
	}
	wantLaunch := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if instance.LaunchTime == nil || !instance.LaunchTime.Equal(wantLaunch) {
		t.Errorf("LaunchTime = %v", instance.LaunchTime)
	}
	if got := string(instance.Tags); got != `{"Name":"web-1","env":"prod"}` {
		t.Errorf("Tags = %q", got)
	}
	if got[1].InstanceID != "i-bbb" || got[2].InstanceID != "i-ccc" {
		t.Errorf("flattened IDs = %q, %q, %q", got[0].InstanceID, got[1].InstanceID, got[2].InstanceID)
	}
}

func TestEC2InstanceToModel_NilAndInvalidFieldsAreSafe(t *testing.T) {
	row := ec2InstanceToModel(ec2types.Instance{
		InstanceId:       aws.String("i-safe"),
		PrivateIpAddress: aws.String("not-an-ip"),
		PublicIpAddress:  aws.String(""),
		Tags: []ec2types.Tag{
			{Key: nil, Value: aws.String("ignored")},
			{Key: aws.String("ignored"), Value: nil},
		},
	})
	if row.InstanceID != "i-safe" {
		t.Fatalf("InstanceID = %q", row.InstanceID)
	}
	if row.AvailabilityZone != "" || row.State != "" || row.LaunchTime != nil {
		t.Fatalf("nil fields mapped incorrectly: %#v", row)
	}
	if row.PrivateIP != nil || row.PublicIP != nil {
		t.Fatalf("invalid IPs must remain nil: private=%v public=%v", row.PrivateIP, row.PublicIP)
	}
	if string(row.Tags) != `{}` {
		t.Fatalf("Tags = %q", row.Tags)
	}
}

func TestEC2Fetcher_SecondPageErrorDiscardsPartialItems(t *testing.T) {
	var requests atomic.Int32
	client := newEC2Client(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			writer.Header().Set("Content-Type", "text/xml")
			_, _ = writer.Write([]byte(describeInstancesPage1))
			return
		}
		http.Error(writer, "upstream failed", http.StatusInternalServerError)
	}))

	items, err := (EC2Fetcher{}).FetchInstances(context.Background(), &Clients{EC2: client})
	if err == nil {
		t.Fatal("expected second-page error")
	}
	if items != nil {
		t.Fatalf("partial items = %#v, want nil", items)
	}
}

func TestEC2Fetcher_RepeatedPaginationTokenDiscardsPartialItems(t *testing.T) {
	responses := []string{
		describeInstancesPage("token-a", "i-first"),
		describeInstancesPage("token-b", "i-second"),
		describeInstancesPage("token-a", "i-third"),
	}
	var requests atomic.Int32
	client := newEC2Client(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		call := int(requests.Add(1))
		if call > len(responses) {
			t.Errorf("request %d proves the repeated token loop was not stopped", call)
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/xml")
		_, _ = writer.Write([]byte(responses[call-1]))
	}))

	items, err := (EC2Fetcher{}).FetchInstances(context.Background(), &Clients{EC2: client})
	if items != nil || !errors.Is(err, ErrEC2PaginationTokenRepeated) {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
}

func describeInstancesPage(nextToken, instanceID string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <nextToken>` + nextToken + `</nextToken>
  <reservationSet><item><instancesSet><item><instanceId>` + instanceID + `</instanceId></item></instancesSet></item></reservationSet>
</DescribeInstancesResponse>`
}

func TestEC2Fetcher_ContextCancellationDiscardsItems(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := newEC2Client(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	// Registered after the server cleanup so LIFO cleanup releases the handler
	// before httptest waits for active connections to close.
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var itemsNil bool
	var fetchErr error
	go func() {
		items, err := (EC2Fetcher{}).FetchInstances(ctx, &Clients{EC2: client})
		itemsNil = items == nil
		fetchErr = err
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FetchInstances did not honor cancellation")
	}
	if !itemsNil || fetchErr == nil || !errors.Is(fetchErr, context.Canceled) {
		t.Fatalf("itemsNil=%v err=%v", itemsNil, fetchErr)
	}
}

func TestEC2Fetcher_NilClientsReturnSafeError(t *testing.T) {
	for name, clients := range map[string]*Clients{"clients": nil, "ec2": {}} {
		t.Run(name, func(t *testing.T) {
			items, err := (EC2Fetcher{}).FetchInstances(context.Background(), clients)
			if items != nil || err == nil {
				t.Fatalf("items=%#v err=%v", items, err)
			}
			if !strings.Contains(err.Error(), "EC2 client") {
				t.Fatalf("unsafe or unclear error: %q", err)
			}
		})
	}
}
