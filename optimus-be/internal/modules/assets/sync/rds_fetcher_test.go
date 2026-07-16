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
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

func TestRDSFetcher_PaginatesAndMapsFields(t *testing.T) {
	var requests atomic.Int32
	client := newRDSClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		call := requests.Add(1)
		if got := request.Form.Get("Action"); got != "DescribeDBInstances" {
			t.Errorf("Action = %q", got)
		}
		if got := request.Form.Get("MaxRecords"); got != "100" {
			t.Errorf("MaxRecords = %q, want 100", got)
		}
		writer.Header().Set("Content-Type", "text/xml")
		switch call {
		case 1:
			if got := request.Form.Get("Marker"); got != "" {
				t.Errorf("first Marker = %q", got)
			}
			_, _ = writer.Write([]byte(describeDBInstancesPage("page-2", `
        <DBInstanceIdentifier>db-prod-1</DBInstanceIdentifier>
        <Engine>postgres</Engine>
        <EngineVersion>16.2</EngineVersion>
        <DBInstanceClass>db.t3.medium</DBInstanceClass>
        <DBInstanceStatus>available</DBInstanceStatus>
        <Endpoint><Address>db-prod-1.example.com</Address><Port>5432</Port></Endpoint>
        <MultiAZ>true</MultiAZ>
        <PubliclyAccessible>false</PubliclyAccessible>
        <AllocatedStorage>100</AllocatedStorage>
        <TagList><Tag><Key>env</Key><Value>prod</Value></Tag><Tag><Key>Name</Key><Value>primary</Value></Tag></TagList>`)))
		case 2:
			if got := request.Form.Get("Marker"); got != "page-2" {
				t.Errorf("second Marker = %q, want page-2", got)
			}
			_, _ = writer.Write([]byte(describeDBInstancesPage("", `<DBInstanceIdentifier>db-replica-1</DBInstanceIdentifier>`)))
		default:
			t.Errorf("unexpected request %d", call)
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))

	got, err := (RDSFetcher{}).FetchDatabases(context.Background(), &Clients{RDS: client})
	if err != nil {
		t.Fatalf("FetchDatabases: %v", err)
	}
	if requests.Load() != 2 || len(got) != 2 {
		t.Fatalf("requests=%d databases=%#v", requests.Load(), got)
	}
	database := got[0]
	if database.DBInstanceID != "db-prod-1" || database.Engine != "postgres" || database.EngineVersion != "16.2" || database.InstanceClass != "db.t3.medium" || database.Status != "available" {
		t.Errorf("identity fields = %#v", database)
	}
	if database.Endpoint != "db-prod-1.example.com" || database.Port == nil || *database.Port != 5432 {
		t.Errorf("endpoint fields = %#v", database)
	}
	if !database.MultiAZ || database.PubliclyAccessible || database.StorageGB == nil || *database.StorageGB != 100 {
		t.Errorf("configuration fields = %#v", database)
	}
	if string(database.Tags) != `{"Name":"primary","env":"prod"}` {
		t.Errorf("Tags = %q", database.Tags)
	}
	if got[1].DBInstanceID != "db-replica-1" {
		t.Errorf("second database = %#v", got[1])
	}
}

func TestRDSDatabaseToModel_NilFieldsAndPointerCopiesAreSafe(t *testing.T) {
	port := int32(5432)
	storage := int32(64)
	input := rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String("db-safe"),
		Endpoint:             &rdstypes.Endpoint{Port: &port},
		AllocatedStorage:     &storage,
		TagList: []rdstypes.Tag{
			{Key: nil, Value: aws.String("ignored")},
			{Key: aws.String("ignored"), Value: nil},
		},
	}
	row := rdsDatabaseToModel(input)
	port, storage = 3306, 128
	if row.DBInstanceID != "db-safe" || row.Endpoint != "" || row.Port == nil || *row.Port != 5432 || row.StorageGB == nil || *row.StorageGB != 64 || string(row.Tags) != `{}` {
		t.Fatalf("database = %#v", row)
	}

	empty := rdsDatabaseToModel(rdstypes.DBInstance{})
	if empty.Endpoint != "" || empty.Port != nil || empty.StorageGB != nil || string(empty.Tags) != `{}` {
		t.Fatalf("empty database = %#v", empty)
	}
}

func TestRDSFetcher_SecondPageErrorDiscardsPartialItems(t *testing.T) {
	var requests atomic.Int32
	client := newRDSClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			writer.Header().Set("Content-Type", "text/xml")
			_, _ = writer.Write([]byte(describeDBInstancesPage("next", `<DBInstanceIdentifier>db-partial</DBInstanceIdentifier>`)))
			return
		}
		http.Error(writer, "upstream failed", http.StatusInternalServerError)
	}))

	items, err := (RDSFetcher{}).FetchDatabases(context.Background(), &Clients{RDS: client})
	if err == nil || items != nil {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestRDSFetcher_FirstPageErrorReturnsNilItems(t *testing.T) {
	client := newRDSClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "upstream failed", http.StatusInternalServerError)
	}))

	items, err := (RDSFetcher{}).FetchDatabases(context.Background(), &Clients{RDS: client})
	if err == nil || items != nil {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestRDSFetcher_RepeatedMarkerDiscardsPartialItems(t *testing.T) {
	markers := []string{"marker-a", "marker-b", "marker-a"}
	var requests atomic.Int32
	client := newRDSClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		call := int(requests.Add(1))
		if call > len(markers) {
			t.Errorf("request %d proves the marker cycle was not stopped", call)
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/xml")
		_, _ = writer.Write([]byte(describeDBInstancesPage(markers[call-1], `<DBInstanceIdentifier>db-partial</DBInstanceIdentifier>`)))
	}))

	items, err := (RDSFetcher{}).FetchDatabases(context.Background(), &Clients{RDS: client})
	if items != nil || !errors.Is(err, ErrRDSPaginationMarkerRepeated) {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
}

func TestRDSFetcher_ContextCancellationIsPropagated(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := newRDSClient(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var itemsNil bool
	var fetchErr error
	go func() {
		got, err := (RDSFetcher{}).FetchDatabases(ctx, &Clients{RDS: client})
		itemsNil = got == nil
		fetchErr = err
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FetchDatabases did not honor cancellation")
	}
	if !itemsNil || !errors.Is(fetchErr, context.Canceled) {
		t.Fatalf("itemsNil=%v err=%v", itemsNil, fetchErr)
	}
}

func TestRDSFetcher_NilClientsReturnStableError(t *testing.T) {
	for name, clients := range map[string]*Clients{"clients": nil, "rds": {}} {
		t.Run(name, func(t *testing.T) {
			items, err := (RDSFetcher{}).FetchDatabases(context.Background(), clients)
			if items != nil || !errors.Is(err, ErrRDSClientRequired) || !strings.Contains(err.Error(), "RDS client") {
				t.Fatalf("items=%#v err=%v", items, err)
			}
		})
	}
}

func newRDSClient(t *testing.T, handler http.Handler) *rds.Client {
	t.Helper()
	server := httptestNewServer(t, handler)
	return rds.NewFromConfig(awsTestConfig(), func(options *rds.Options) {
		options.BaseEndpoint = aws.String(server.URL)
	})
}

func describeDBInstancesPage(marker, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<DescribeDBInstancesResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
  <DescribeDBInstancesResult>
    <DBInstances><DBInstance>` + body + `</DBInstance></DBInstances>
    <Marker>` + marker + `</Marker>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`
}
