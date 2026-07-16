package sync

import (
	"context"

	"optimus-be/internal/models"
)

// CompositeFetcher routes each resource sweep to its AWS service fetcher.
type CompositeFetcher struct {
	Instance interface {
		FetchInstances(context.Context, *Clients) ([]models.AWSInstance, error)
	}
	Network interface {
		FetchVPCsAndSubnets(context.Context, *Clients) ([]models.AWSVPC, []models.AWSSubnet, error)
	}
	Database interface {
		FetchDatabases(context.Context, *Clients) ([]models.AWSDatabase, error)
	}
}

func (fetcher *CompositeFetcher) FetchInstances(ctx context.Context, clients *Clients) ([]models.AWSInstance, error) {
	if fetcher == nil || fetcher.Instance == nil {
		return nil, nil
	}
	return fetcher.Instance.FetchInstances(ctx, clients)
}

func (fetcher *CompositeFetcher) FetchVPCsAndSubnets(ctx context.Context, clients *Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	if fetcher == nil || fetcher.Network == nil {
		return nil, nil, nil
	}
	return fetcher.Network.FetchVPCsAndSubnets(ctx, clients)
}

func (fetcher *CompositeFetcher) FetchDatabases(ctx context.Context, clients *Clients) ([]models.AWSDatabase, error) {
	if fetcher == nil || fetcher.Database == nil {
		return nil, nil
	}
	return fetcher.Database.FetchDatabases(ctx, clients)
}
