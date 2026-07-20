package sync

import (
	"context"
	"errors"

	"optimus-be/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"gorm.io/datatypes"
)

// ErrRDSClientRequired is returned when a database sweep has no RDS client.
var ErrRDSClientRequired = errors.New("assets sync: RDS client is required")

// ErrRDSPaginationMarkerRepeated indicates a non-authoritative response cycle.
// The caller must discard all databases fetched before this error.
var ErrRDSPaginationMarkerRepeated = errors.New("assets sync: RDS pagination marker repeated")

type RDSFetcher struct{}

// FetchDatabases returns an authoritative region snapshot only after every
// DescribeDBInstances page has been fetched successfully.
func (RDSFetcher) FetchDatabases(ctx context.Context, clients *Clients) ([]models.AWSDatabase, error) {
	if clients == nil || clients.RDS == nil {
		return nil, ErrRDSClientRequired
	}

	var out []models.AWSDatabase
	seenMarkers := make(map[string]struct{})
	paginator := rds.NewDescribeDBInstancesPaginator(clients.RDS, &rds.DescribeDBInstancesInput{
		MaxRecords: aws.Int32(100),
	}, func(options *rds.DescribeDBInstancesPaginatorOptions) {
		// Repeated markers are errors below, never an authoritative early stop.
		options.StopOnDuplicateToken = false
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, database := range page.DBInstances {
			out = append(out, rdsDatabaseToModel(database))
		}

		nextMarker := aws.ToString(page.Marker)
		if nextMarker != "" {
			if _, seen := seenMarkers[nextMarker]; seen {
				return nil, ErrRDSPaginationMarkerRepeated
			}
			seenMarkers[nextMarker] = struct{}{}
		}
	}
	return out, nil
}

func rdsDatabaseToModel(database rdstypes.DBInstance) models.AWSDatabase {
	row := models.AWSDatabase{
		DBInstanceID:       aws.ToString(database.DBInstanceIdentifier),
		Engine:             aws.ToString(database.Engine),
		EngineVersion:      aws.ToString(database.EngineVersion),
		InstanceClass:      aws.ToString(database.DBInstanceClass),
		Status:             aws.ToString(database.DBInstanceStatus),
		MultiAZ:            aws.ToBool(database.MultiAZ),
		PubliclyAccessible: aws.ToBool(database.PubliclyAccessible),
		Tags:               datatypes.JSON(RDSTagJSON(database.TagList)),
	}
	if database.Endpoint != nil {
		row.Endpoint = aws.ToString(database.Endpoint.Address)
		if database.Endpoint.Port != nil {
			port := *database.Endpoint.Port
			row.Port = &port
		}
	}
	if database.AllocatedStorage != nil {
		storage := *database.AllocatedStorage
		row.StorageGB = &storage
	}
	return row
}
