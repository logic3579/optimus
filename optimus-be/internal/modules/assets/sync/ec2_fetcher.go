package sync

import (
	"context"
	"errors"
	"net"

	"optimus-be/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"gorm.io/datatypes"
)

var errEC2ClientRequired = errors.New("assets sync: EC2 client is required")

// ErrEC2PaginationTokenRepeated indicates a non-authoritative response cycle.
// The caller must discard all items fetched before this error.
var ErrEC2PaginationTokenRepeated = errors.New("assets sync: EC2 pagination token repeated")

type EC2Fetcher struct{}

// FetchInstances returns an authoritative region snapshot only after every
// DescribeInstances page has been fetched successfully.
func (EC2Fetcher) FetchInstances(ctx context.Context, clients *Clients) ([]models.AWSInstance, error) {
	if clients == nil || clients.EC2 == nil {
		return nil, errEC2ClientRequired
	}

	var out []models.AWSInstance
	seenTokens := make(map[string]struct{})
	paginator := ec2.NewDescribeInstancesPaginator(clients.EC2, &ec2.DescribeInstancesInput{
		MaxResults: aws.Int32(100),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		if token := aws.ToString(page.NextToken); token != "" {
			if _, seen := seenTokens[token]; seen {
				return nil, ErrEC2PaginationTokenRepeated
			}
			seenTokens[token] = struct{}{}
		}
		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				out = append(out, ec2InstanceToModel(instance))
			}
		}
	}
	return out, nil
}

func ec2InstanceToModel(instance ec2types.Instance) models.AWSInstance {
	row := models.AWSInstance{
		InstanceID:   aws.ToString(instance.InstanceId),
		InstanceType: string(instance.InstanceType),
		Name:         EC2TagName(instance.Tags),
		VPCID:        aws.ToString(instance.VpcId),
		SubnetID:     aws.ToString(instance.SubnetId),
		Tags:         datatypes.JSON(EC2TagJSON(instance.Tags)),
	}
	if instance.State != nil {
		row.State = string(instance.State.Name)
	}
	if instance.Placement != nil {
		row.AvailabilityZone = aws.ToString(instance.Placement.AvailabilityZone)
	}
	if privateIP := parseIP(instance.PrivateIpAddress); privateIP != nil {
		row.PrivateIP = privateIP
	}
	if publicIP := parseIP(instance.PublicIpAddress); publicIP != nil {
		row.PublicIP = publicIP
	}
	if instance.LaunchTime != nil {
		launchTime := *instance.LaunchTime
		row.LaunchTime = &launchTime
	}
	return row
}

func parseIP(value *string) *net.IP {
	if value == nil {
		return nil
	}
	parsed := net.ParseIP(*value)
	if parsed == nil {
		return nil
	}
	return &parsed
}
