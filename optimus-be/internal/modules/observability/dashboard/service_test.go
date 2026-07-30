package dashboard

import (
	"math"
	"strconv"
	"testing"
)

func TestValidateSaveRequestRejectsInvalidAggregate(t *testing.T) {
	valid := SaveRequest{Name: "ops", RefreshIntervalS: 30, TimeRange: "1h", Panels: []PanelInput{{DatasourceID: 1, Title: "CPU", PanelType: "time_series", PromQL: "up", Unit: "percent", SortOrder: 0, Width: 12}}}
	cases := map[string]func(*SaveRequest){
		"duplicate order": func(r *SaveRequest) { r.Panels = append(r.Panels, r.Panels[0]) },
		"unit":            func(r *SaveRequest) { r.Panels[0].Unit = "javascript" },
		"type":            func(r *SaveRequest) { r.Panels[0].PanelType = "gauge" },
		"width":           func(r *SaveRequest) { r.Panels[0].Width = 9 },
		"promql":          func(r *SaveRequest) { r.Panels[0].PromQL = "" },
		"refresh":         func(r *SaveRequest) { r.RefreshIntervalS = 14 },
		"range":           func(r *SaveRequest) { r.TimeRange = "30d" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := valid
			r.Panels = append([]PanelInput(nil), valid.Panels...)
			mutate(&r)
			if err := validateSave(r); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateSaveRequestBoundsAggregateAndDatabaseIntegers(t *testing.T) {
	panel := PanelInput{DatasourceID: 1, Title: "CPU", PanelType: "stat", PromQL: "up", Unit: "none", SortOrder: 0, Width: 12}
	r := SaveRequest{Name: "ops", RefreshIntervalS: 30, TimeRange: "1h", Panels: make([]PanelInput, 101)}
	for i := range r.Panels {
		r.Panels[i] = panel
		r.Panels[i].SortOrder = i
	}
	if err := validateSave(r); err == nil {
		t.Fatal("expected panel count limit")
	}
	r.Panels = []PanelInput{panel}
	r.Panels[0].DatasourceID = uint64(math.MaxInt64)
	if err := validateSave(r); err != nil {
		t.Fatalf("BIGINT boundary rejected: %v", err)
	}
	r.Panels[0].DatasourceID++
	if err := validateSave(r); err == nil {
		t.Fatal("expected BIGINT overflow rejection")
	}
	r.Panels[0] = panel
	r.Panels[0].SortOrder = math.MaxInt32
	if err := validateSave(r); err != nil {
		t.Fatalf("INTEGER boundary rejected: %v", err)
	}
	if strconv.IntSize > 32 {
		r.Panels[0].SortOrder = int(int64(math.MaxInt32) + 1)
		if err := validateSave(r); err == nil {
			t.Fatal("expected INTEGER overflow rejection")
		}
	}
}

func TestPanelFingerprintsExcludePromQL(t *testing.T) {
	p := []PanelInput{{DatasourceID: 1, Title: "CPU", PanelType: "stat", PromQL: "secret_tenant_query", Unit: "none", SortOrder: 0, Width: 6}}
	f := panelFingerprints(p)
	if len(f) != 1 || len(f[0]) != 64 {
		t.Fatalf("unexpected fingerprints: %#v", f)
	}
	if f[0] == p[0].PromQL {
		t.Fatal("full PromQL leaked")
	}
}

func TestPanelFingerprintsRemainBoundedAtAggregateLimit(t *testing.T) {
	panels := make([]PanelInput, 100)
	for i := range panels {
		panels[i] = PanelInput{DatasourceID: 1, Title: "panel", PanelType: "stat", PromQL: "tenant_query", Unit: "none", SortOrder: i, Width: 12}
	}
	fingerprints := panelFingerprints(panels)
	if len(fingerprints) != 100 {
		t.Fatalf("fingerprints=%d", len(fingerprints))
	}
	for _, fingerprint := range fingerprints {
		if len(fingerprint) != 64 {
			t.Fatalf("unbounded fingerprint=%q", fingerprint)
		}
	}
}
