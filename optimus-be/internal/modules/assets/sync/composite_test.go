package sync

import (
	"context"
	"errors"
	"testing"

	"optimus-be/internal/models"
)

type instanceFetcherFunc func(context.Context, *Clients) ([]models.AWSInstance, error)

func (fn instanceFetcherFunc) FetchInstances(ctx context.Context, clients *Clients) ([]models.AWSInstance, error) {
	return fn(ctx, clients)
}

type networkFetcherFunc func(context.Context, *Clients) ([]models.AWSVPC, []models.AWSSubnet, error)

func (fn networkFetcherFunc) FetchVPCsAndSubnets(ctx context.Context, clients *Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	return fn(ctx, clients)
}

type databaseFetcherFunc func(context.Context, *Clients) ([]models.AWSDatabase, error)

func (fn databaseFetcherFunc) FetchDatabases(ctx context.Context, clients *Clients) ([]models.AWSDatabase, error) {
	return fn(ctx, clients)
}

func TestCompositeFetcher_RoutesComponents(t *testing.T) {
	wantErr := errors.New("database failed")
	clients := &Clients{}
	fetcher := &CompositeFetcher{
		Instance: instanceFetcherFunc(func(_ context.Context, got *Clients) ([]models.AWSInstance, error) {
			if got != clients {
				t.Error("instance client was not forwarded")
			}
			return []models.AWSInstance{{InstanceID: "i-1"}}, nil
		}),
		Network: networkFetcherFunc(func(_ context.Context, got *Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
			if got != clients {
				t.Error("network client was not forwarded")
			}
			return []models.AWSVPC{{VPCID: "vpc-1"}}, []models.AWSSubnet{{SubnetID: "subnet-1"}}, nil
		}),
		Database: databaseFetcherFunc(func(_ context.Context, got *Clients) ([]models.AWSDatabase, error) {
			if got != clients {
				t.Error("database client was not forwarded")
			}
			return nil, wantErr
		}),
	}

	instances, err := fetcher.FetchInstances(context.Background(), clients)
	if err != nil || len(instances) != 1 || instances[0].InstanceID != "i-1" {
		t.Fatalf("FetchInstances() = %#v, %v", instances, err)
	}
	vpcs, subnets, err := fetcher.FetchVPCsAndSubnets(context.Background(), clients)
	if err != nil || len(vpcs) != 1 || len(subnets) != 1 {
		t.Fatalf("FetchVPCsAndSubnets() = %#v, %#v, %v", vpcs, subnets, err)
	}
	if databases, err := fetcher.FetchDatabases(context.Background(), clients); databases != nil || !errors.Is(err, wantErr) {
		t.Fatalf("FetchDatabases() = %#v, %v", databases, err)
	}
}

func TestCompositeFetcher_MissingComponentsReturnConfigurationError(t *testing.T) {
	tests := []struct {
		name    string
		fetcher *CompositeFetcher
	}{
		{name: "nil receiver"},
		{name: "nil components", fetcher: &CompositeFetcher{}},
		{name: "typed nil components", fetcher: &CompositeFetcher{
			Instance: instanceFetcherFunc(nil),
			Network:  networkFetcherFunc(nil),
			Database: databaseFetcherFunc(nil),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instances, err := test.fetcher.FetchInstances(context.Background(), nil)
			if instances != nil || !errors.Is(err, ErrFetcherNotConfigured) {
				t.Errorf("FetchInstances() = %#v, %v", instances, err)
			}

			vpcs, subnets, err := test.fetcher.FetchVPCsAndSubnets(context.Background(), nil)
			if vpcs != nil || subnets != nil || !errors.Is(err, ErrFetcherNotConfigured) {
				t.Errorf("FetchVPCsAndSubnets() = %#v, %#v, %v", vpcs, subnets, err)
			}

			databases, err := test.fetcher.FetchDatabases(context.Background(), nil)
			if databases != nil || !errors.Is(err, ErrFetcherNotConfigured) {
				t.Errorf("FetchDatabases() = %#v, %v", databases, err)
			}
		})
	}
}
