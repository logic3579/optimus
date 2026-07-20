package sync

import (
	"context"

	"optimus-be/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"gorm.io/datatypes"
)

type VPCFetcher struct{}

// FetchVPCsAndSubnets returns one authoritative network snapshot only after
// both APIs have completed pagination successfully.
func (VPCFetcher) FetchVPCsAndSubnets(ctx context.Context, clients *Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	if clients == nil || clients.EC2 == nil {
		return nil, nil, errEC2ClientRequired
	}

	vpcs, err := fetchVPCs(ctx, clients.EC2)
	if err != nil {
		return nil, nil, err
	}
	subnets, err := fetchSubnets(ctx, clients.EC2)
	if err != nil {
		return nil, nil, err
	}
	return vpcs, subnets, nil
}

func fetchVPCs(ctx context.Context, client *ec2.Client) ([]models.AWSVPC, error) {
	var out []models.AWSVPC
	seenTokens := make(map[string]struct{})
	paginator := ec2.NewDescribeVpcsPaginator(client, &ec2.DescribeVpcsInput{
		MaxResults: aws.Int32(100),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		if paginationTokenRepeated(page.NextToken, seenTokens) {
			return nil, ErrEC2PaginationTokenRepeated
		}
		for _, vpc := range page.Vpcs {
			out = append(out, vpcToModel(vpc))
		}
	}
	return out, nil
}

func fetchSubnets(ctx context.Context, client *ec2.Client) ([]models.AWSSubnet, error) {
	var out []models.AWSSubnet
	seenTokens := make(map[string]struct{})
	paginator := ec2.NewDescribeSubnetsPaginator(client, &ec2.DescribeSubnetsInput{
		MaxResults: aws.Int32(100),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		if paginationTokenRepeated(page.NextToken, seenTokens) {
			return nil, ErrEC2PaginationTokenRepeated
		}
		for _, subnet := range page.Subnets {
			out = append(out, subnetToModel(subnet))
		}
	}
	return out, nil
}

func paginationTokenRepeated(token *string, seen map[string]struct{}) bool {
	value := aws.ToString(token)
	if value == "" {
		return false
	}
	if _, ok := seen[value]; ok {
		return true
	}
	seen[value] = struct{}{}
	return false
}

func vpcToModel(vpc ec2types.Vpc) models.AWSVPC {
	row := models.AWSVPC{
		VPCID:     aws.ToString(vpc.VpcId),
		Name:      EC2TagName(vpc.Tags),
		IsDefault: aws.ToBool(vpc.IsDefault),
		State:     string(vpc.State),
		Tags:      datatypes.JSON(EC2TagJSON(vpc.Tags)),
	}
	if cidr := aws.ToString(vpc.CidrBlock); cidr != "" {
		row.CIDRBlock = &cidr
	}
	return row
}

func subnetToModel(subnet ec2types.Subnet) models.AWSSubnet {
	row := models.AWSSubnet{
		SubnetID:         aws.ToString(subnet.SubnetId),
		VPCID:            aws.ToString(subnet.VpcId),
		AvailabilityZone: aws.ToString(subnet.AvailabilityZone),
		Name:             EC2TagName(subnet.Tags),
		Tags:             datatypes.JSON(EC2TagJSON(subnet.Tags)),
	}
	if cidr := aws.ToString(subnet.CidrBlock); cidr != "" {
		row.CIDRBlock = &cidr
	}
	return row
}
