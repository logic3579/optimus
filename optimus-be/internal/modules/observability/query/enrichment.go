package query

import (
	"context"
	"log/slog"
	"net/netip"
	"sort"

	"optimus-be/internal/modules/assets"
)

var enrichmentLabels = []string{"private_ip", "instance_ip", "node_ip"}

func enrich(ctx context.Context, consumer assets.Consumer, items []ItemResult, limit int) map[string]AssetSummary {
	if consumer == nil || limit <= 0 {
		return nil
	}
	set := map[netip.Addr]struct{}{}
	for _, item := range items {
		if item.Result == nil {
			continue
		}
		for _, series := range item.Result.Series {
			for _, label := range enrichmentLabels {
				if raw := series.Labels[label]; raw != "" {
					if ip, err := netip.ParseAddr(raw); err == nil {
						set[ip.Unmap()] = struct{}{}
					}
				}
			}
		}
	}
	ips := make([]netip.Addr, 0, len(set))
	for ip := range set {
		ips = append(ips, ip)
	}
	sort.Slice(ips, func(i, j int) bool { return ips[i].Compare(ips[j]) < 0 })
	if len(ips) > limit {
		ips = ips[:limit]
	}
	out := map[string]AssetSummary{}
	for _, ip := range ips {
		if ctx.Err() != nil {
			break
		}
		row, err := consumer.LookupInstanceByPrivateIP(ctx, ip)
		if err != nil {
			slog.Debug("asset enrichment lookup skipped", "ip", ip.String(), "error_class", "lookup_failed")
			continue
		}
		v := AssetSummary{InstanceID: row.InstanceID, Name: row.Name, AccountID: row.AccountID, AccountName: row.AccountName, Region: row.Region, InstanceType: row.InstanceType, State: row.State, PrivateIP: row.PrivateIP.String(), VPCID: row.VPCID, SubnetID: row.SubnetID}
		if row.PublicIP.IsValid() {
			v.PublicIP = row.PublicIP.String()
		}
		out[ip.String()] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
