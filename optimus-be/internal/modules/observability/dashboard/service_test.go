package dashboard

import "testing"

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
