package sync

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestVPCFetcher_PaginatesBothAPIsAndMapsFields(t *testing.T) {
	var mu sync.Mutex
	var vpcCalls, subnetCalls int
	client := newEC2Client(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		writer.Header().Set("Content-Type", "text/xml")
		switch request.Form.Get("Action") {
		case "DescribeVpcs":
			vpcCalls++
			assertPaginationRequest(t, request, vpcCalls, "vpc-page-2")
			if vpcCalls == 1 {
				_, _ = writer.Write([]byte(describeVPCPage("vpc-page-2", "vpc-1", "10.0.0.0/16", "main-vpc", true, "available")))
			} else {
				_, _ = writer.Write([]byte(describeVPCPage("", "vpc-2", "", "", false, "pending")))
			}
		case "DescribeSubnets":
			subnetCalls++
			assertPaginationRequest(t, request, subnetCalls, "subnet-page-2")
			if subnetCalls == 1 {
				_, _ = writer.Write([]byte(describeSubnetPage("subnet-page-2", "subnet-1", "vpc-1", "10.0.1.0/24", "us-east-1a", "web-subnet")))
			} else {
				_, _ = writer.Write([]byte(describeSubnetPage("", "subnet-2", "vpc-2", "", "us-east-1b", "")))
			}
		default:
			http.Error(writer, "unexpected action", http.StatusInternalServerError)
		}
	}))

	vpcs, subnets, err := (VPCFetcher{}).FetchVPCsAndSubnets(context.Background(), &Clients{EC2: client})
	if err != nil {
		t.Fatalf("FetchVPCsAndSubnets: %v", err)
	}
	if vpcCalls != 2 || subnetCalls != 2 {
		t.Fatalf("calls = vpcs:%d subnets:%d, want 2 each", vpcCalls, subnetCalls)
	}
	if len(vpcs) != 2 || len(subnets) != 2 {
		t.Fatalf("items = vpcs:%#v subnets:%#v", vpcs, subnets)
	}
	if got := vpcs[0]; got.VPCID != "vpc-1" || got.CIDRBlock == nil || *got.CIDRBlock != "10.0.0.0/16" || got.Name != "main-vpc" || !got.IsDefault || got.State != "available" || string(got.Tags) != `{"Name":"main-vpc","env":"prod"}` {
		t.Errorf("first VPC = %#v", got)
	}
	if got := vpcs[1]; got.VPCID != "vpc-2" || got.CIDRBlock != nil || got.Name != "" || got.IsDefault || got.State != "pending" || string(got.Tags) != `{"Name":"","env":"prod"}` {
		t.Errorf("second VPC = %#v", got)
	}
	if got := subnets[0]; got.SubnetID != "subnet-1" || got.VPCID != "vpc-1" || got.CIDRBlock == nil || *got.CIDRBlock != "10.0.1.0/24" || got.AvailabilityZone != "us-east-1a" || got.Name != "web-subnet" || string(got.Tags) != `{"Name":"web-subnet","env":"prod"}` {
		t.Errorf("first subnet = %#v", got)
	}
	if got := subnets[1]; got.SubnetID != "subnet-2" || got.VPCID != "vpc-2" || got.CIDRBlock != nil || got.AvailabilityZone != "us-east-1b" || got.Name != "" || string(got.Tags) != `{"Name":"","env":"prod"}` {
		t.Errorf("second subnet = %#v", got)
	}
}

func assertPaginationRequest(t *testing.T, request *http.Request, call int, secondToken string) {
	t.Helper()
	if got := request.Form.Get("MaxResults"); got != "100" {
		t.Errorf("MaxResults = %q, want 100", got)
	}
	wantToken := ""
	if call == 2 {
		wantToken = secondToken
	}
	if got := request.Form.Get("NextToken"); got != wantToken {
		t.Errorf("call %d NextToken = %q, want %q", call, got, wantToken)
	}
}

func TestVPCModelHelpers_NilPointersAndTagsAreSafe(t *testing.T) {
	vpc := vpcToModel(ec2types.Vpc{Tags: []ec2types.Tag{{Key: nil, Value: aws.String("ignored")}, {Key: aws.String("ignored"), Value: nil}}})
	if vpc.VPCID != "" || vpc.CIDRBlock != nil || vpc.Name != "" || vpc.IsDefault || vpc.State != "" || string(vpc.Tags) != `{}` {
		t.Fatalf("VPC = %#v", vpc)
	}
	subnet := subnetToModel(ec2types.Subnet{Tags: []ec2types.Tag{{Key: nil, Value: nil}}})
	if subnet.SubnetID != "" || subnet.VPCID != "" || subnet.CIDRBlock != nil || subnet.AvailabilityZone != "" || subnet.Name != "" || string(subnet.Tags) != `{}` {
		t.Fatalf("subnet = %#v", subnet)
	}
}

func TestVPCFetcher_VPCFailuresAreFailFastAndDiscardBothSets(t *testing.T) {
	tests := map[string]func(t *testing.T, writer http.ResponseWriter, request *http.Request, call int){
		"page error": func(_ *testing.T, writer http.ResponseWriter, _ *http.Request, call int) {
			if call == 1 {
				writer.Header().Set("Content-Type", "text/xml")
				_, _ = writer.Write([]byte(describeVPCPage("next", "vpc-partial", "10.0.0.0/16", "", false, "available")))
				return
			}
			http.Error(writer, "failed", http.StatusInternalServerError)
		},
		"repeated token": func(_ *testing.T, writer http.ResponseWriter, _ *http.Request, call int) {
			tokens := []string{"a", "b", "a"}
			writer.Header().Set("Content-Type", "text/xml")
			_, _ = writer.Write([]byte(describeVPCPage(tokens[call-1], "vpc-partial", "", "", false, "available")))
		},
	}
	for name, respond := range tests {
		t.Run(name, func(t *testing.T) {
			var vpcCalls, subnetCalls atomic.Int32
			client := newEC2Client(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_ = request.ParseForm()
				if request.Form.Get("Action") == "DescribeSubnets" {
					subnetCalls.Add(1)
					http.Error(writer, "must not be called", http.StatusInternalServerError)
					return
				}
				call := int(vpcCalls.Add(1))
				respond(t, writer, request, call)
			}))
			vpcs, subnets, err := (VPCFetcher{}).FetchVPCsAndSubnets(context.Background(), &Clients{EC2: client})
			if err == nil || vpcs != nil || subnets != nil {
				t.Fatalf("vpcs=%#v subnets=%#v err=%v", vpcs, subnets, err)
			}
			if subnetCalls.Load() != 0 {
				t.Fatalf("DescribeSubnets calls = %d, want 0", subnetCalls.Load())
			}
			if name == "repeated token" && !errors.Is(err, ErrEC2PaginationTokenRepeated) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestVPCFetcher_SubnetFailuresDiscardCompleteVPCsAndPartialSubnets(t *testing.T) {
	tests := map[string]func(writer http.ResponseWriter, call int){
		"page error": func(writer http.ResponseWriter, call int) {
			if call == 1 {
				writer.Header().Set("Content-Type", "text/xml")
				_, _ = writer.Write([]byte(describeSubnetPage("next", "subnet-partial", "vpc-1", "", "", "")))
				return
			}
			http.Error(writer, "failed", http.StatusInternalServerError)
		},
		"repeated token": func(writer http.ResponseWriter, call int) {
			tokens := []string{"a", "b", "a"}
			writer.Header().Set("Content-Type", "text/xml")
			_, _ = writer.Write([]byte(describeSubnetPage(tokens[call-1], "subnet-partial", "vpc-1", "", "", "")))
		},
	}
	for name, respond := range tests {
		t.Run(name, func(t *testing.T) {
			var subnetCalls atomic.Int32
			client := newEC2Client(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_ = request.ParseForm()
				if request.Form.Get("Action") == "DescribeVpcs" {
					writer.Header().Set("Content-Type", "text/xml")
					_, _ = writer.Write([]byte(describeVPCPage("", "vpc-complete", "", "", false, "available")))
					return
				}
				respond(writer, int(subnetCalls.Add(1)))
			}))
			vpcs, subnets, err := (VPCFetcher{}).FetchVPCsAndSubnets(context.Background(), &Clients{EC2: client})
			if err == nil || vpcs != nil || subnets != nil {
				t.Fatalf("vpcs=%#v subnets=%#v err=%v", vpcs, subnets, err)
			}
			if name == "repeated token" && !errors.Is(err, ErrEC2PaginationTokenRepeated) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestVPCFetcher_ContextCancellationAtEitherStageDiscardsBothSets(t *testing.T) {
	for _, stage := range []string{"vpc", "subnet"} {
		t.Run(stage, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			client := newEC2Client(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_ = request.ParseForm()
				action := request.Form.Get("Action")
				if stage == "subnet" && action == "DescribeVpcs" {
					writer.Header().Set("Content-Type", "text/xml")
					_, _ = writer.Write([]byte(describeVPCPage("", "vpc-complete", "", "", false, "available")))
					return
				}
				once.Do(func() { close(started) })
				select {
				case <-request.Context().Done():
				case <-release:
				}
			}))
			t.Cleanup(func() { close(release) })
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			var vpcsNil, subnetsNil bool
			var fetchErr error
			go func() {
				vpcs, subnets, err := (VPCFetcher{}).FetchVPCsAndSubnets(ctx, &Clients{EC2: client})
				vpcsNil, subnetsNil, fetchErr = vpcs == nil, subnets == nil, err
				close(done)
			}()
			<-started
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("fetch did not honor cancellation")
			}
			if !vpcsNil || !subnetsNil || !errors.Is(fetchErr, context.Canceled) {
				t.Fatalf("vpcsNil=%v subnetsNil=%v err=%v", vpcsNil, subnetsNil, fetchErr)
			}
		})
	}
}

func TestVPCFetcher_NilClientsReturnSafeError(t *testing.T) {
	for name, clients := range map[string]*Clients{"clients": nil, "ec2": {}} {
		t.Run(name, func(t *testing.T) {
			vpcs, subnets, err := (VPCFetcher{}).FetchVPCsAndSubnets(context.Background(), clients)
			if vpcs != nil || subnets != nil || err == nil || !strings.Contains(err.Error(), "EC2 client") {
				t.Fatalf("vpcs=%#v subnets=%#v err=%v", vpcs, subnets, err)
			}
		})
	}
}

func describeVPCPage(nextToken, id, cidr, name string, isDefault bool, state string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<DescribeVpcsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <nextToken>` + nextToken + `</nextToken><vpcSet><item><vpcId>` + id + `</vpcId><cidrBlock>` + cidr + `</cidrBlock><isDefault>` + boolText(isDefault) + `</isDefault><state>` + state + `</state><tagSet><item><key>Name</key><value>` + name + `</value></item><item><key>env</key><value>prod</value></item></tagSet></item></vpcSet>
</DescribeVpcsResponse>`
}

func describeSubnetPage(nextToken, id, vpcID, cidr, zone, name string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<DescribeSubnetsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <nextToken>` + nextToken + `</nextToken><subnetSet><item><subnetId>` + id + `</subnetId><vpcId>` + vpcID + `</vpcId><cidrBlock>` + cidr + `</cidrBlock><availabilityZone>` + zone + `</availabilityZone><tagSet><item><key>Name</key><value>` + name + `</value></item><item><key>env</key><value>prod</value></item></tagSet></item></subnetSet>
</DescribeSubnetsResponse>`
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
