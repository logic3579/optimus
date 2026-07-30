package query

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/modules/assets"
	"optimus-be/internal/modules/observability/prometheus"
)

type fakeAssets struct {
	seen []netip.Addr
	rows map[string]*assets.Instance
	err  error
}

func (f *fakeAssets) LookupInstanceByPrivateIP(_ context.Context, ip netip.Addr) (*assets.Instance, error) {
	f.seen = append(f.seen, ip)
	if f.err != nil {
		return nil, f.err
	}
	v := f.rows[ip.String()]
	if v == nil {
		return nil, assets.ErrAssetsInstanceNotFound
	}
	return v, nil
}
func (f *fakeAssets) LookupInstanceByID(context.Context, int64, string, string) (*assets.Instance, error) {
	return nil, nil
}
func (f *fakeAssets) ListInstancesByVPC(context.Context, int64, string, string) ([]assets.Instance, error) {
	return nil, nil
}
func TestEnrichConfiguredLabelsUnmapsDedupesSortsAndDoesNotMutate(t *testing.T) {
	a := &fakeAssets{rows: map[string]*assets.Instance{"10.0.0.1": {InstanceID: "i-1", Name: "one"}, "10.0.0.2": {InstanceID: "i-2"}}}
	labels := map[string]string{"private_ip": "::ffff:10.0.0.2", "instance_ip": "10.0.0.1", "ignored": "10.0.0.3"}
	result := []ItemResult{{Result: &prometheus.Result{Series: []prometheus.Series{{Labels: labels}, {Labels: map[string]string{"node_ip": "10.0.0.1"}}}}}}
	got := enrich(context.Background(), a, result, 100)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2")}, a.seen)
	require.Len(t, got, 2)
	require.Equal(t, "::ffff:10.0.0.2", labels["private_ip"])
}
func TestEnrichCapsAndIgnoresErrors(t *testing.T) {
	a := &fakeAssets{err: errors.New("db")}
	series := make([]prometheus.Series, 101)
	for i := range series {
		series[i].Labels = map[string]string{"private_ip": netip.AddrFrom4([4]byte{10, 0, byte(i / 255), byte(i % 255)}).String()}
	}
	got := enrich(context.Background(), a, []ItemResult{{Result: &prometheus.Result{Series: series}}}, 100)
	require.Empty(t, got)
	require.Len(t, a.seen, 100)
}
