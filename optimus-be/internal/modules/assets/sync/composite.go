package sync

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"optimus-be/internal/models"
)

// ErrFetcherNotConfigured prevents a missing component from being mistaken
// for an authoritative empty cloud snapshot.
var ErrFetcherNotConfigured = errors.New("assets sync fetcher is not configured")

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
	if fetcher == nil || isNilFetcher(fetcher.Instance) {
		return nil, fmt.Errorf("instance: %w", ErrFetcherNotConfigured)
	}
	return fetcher.Instance.FetchInstances(ctx, clients)
}

func (fetcher *CompositeFetcher) FetchVPCsAndSubnets(ctx context.Context, clients *Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	if fetcher == nil || isNilFetcher(fetcher.Network) {
		return nil, nil, fmt.Errorf("network: %w", ErrFetcherNotConfigured)
	}
	return fetcher.Network.FetchVPCsAndSubnets(ctx, clients)
}

func (fetcher *CompositeFetcher) FetchDatabases(ctx context.Context, clients *Clients) ([]models.AWSDatabase, error) {
	if fetcher == nil || isNilFetcher(fetcher.Database) {
		return nil, fmt.Errorf("database: %w", ErrFetcherNotConfigured)
	}
	return fetcher.Database.FetchDatabases(ctx, clients)
}

func isNilFetcher(fetcher any) bool {
	if fetcher == nil {
		return true
	}
	value := reflect.ValueOf(fetcher)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
