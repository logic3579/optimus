package builtin

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

type Variable struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}
type Panel struct {
	RefID     string `json:"ref_id"`
	TitleKey  string `json:"title_key"`
	PanelType string `json:"panel_type"`
	PromQL    string `json:"promql"`
	Unit      string `json:"unit"`
	SortOrder int    `json:"sort_order"`
	Width     int    `json:"width"`
}
type Dashboard struct {
	Code      string     `json:"code"`
	TitleKey  string     `json:"title_key"`
	Variables []Variable `json:"variables"`
	Panels    []Panel    `json:"panels"`
}

func p(ref, title, kind, q, unit string, order, width int) Panel {
	return Panel{ref, title, kind, q, unit, order, width}
}

var definitions = []Dashboard{
	{Code: "kubernetes-cluster", TitleKey: "observability.builtin.kubernetes_cluster", Panels: []Panel{
		p("cpu", "observability.panel.cluster_cpu", "time_series", `sum(rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m]))`, `cores`, 0, 6),
		p("memory", "observability.panel.cluster_memory", "time_series", `sum(container_memory_working_set_bytes{container!="",image!=""})`, `bytes`, 1, 6),
		p("nodes-ready", "observability.panel.nodes_ready", "stat", `sum(kube_node_status_condition{condition="Ready",status="true"})`, `none`, 2, 6),
		p("pods-phase", "observability.panel.pods_phase", "table", `sum by (phase) (kube_pod_status_phase)`, `none`, 3, 6),
		p("restart-rate", "observability.panel.restart_rate", "time_series", `sum(rate(kube_pod_container_status_restarts_total[5m]))`, `requests_per_second`, 4, 12),
	}},
	{Code: "kubernetes-nodes", TitleKey: "observability.builtin.kubernetes_nodes", Variables: []Variable{{Name: "node", Label: "node"}}, Panels: []Panel{
		p("node-cpu", "observability.panel.node_cpu", "time_series", `sum by (node) (rate(container_cpu_usage_seconds_total{node=~"${node}",container!=""}[5m]))`, `cores`, 0, 6),
		p("node-memory", "observability.panel.node_memory", "time_series", `sum by (node) (container_memory_working_set_bytes{node=~"${node}",container!=""})`, `bytes`, 1, 6),
		p("node-ready", "observability.panel.node_ready", "table", `kube_node_status_condition{node=~"${node}",condition="Ready",status="true"}`, `none`, 2, 12),
	}},
	{Code: "kubernetes-workloads", TitleKey: "observability.builtin.kubernetes_workloads", Variables: []Variable{{Name: "namespace", Label: "namespace", Required: true}, {Name: "workload", Label: "workload"}}, Panels: []Panel{
		p("workload-cpu", "observability.panel.workload_cpu", "time_series", `sum by (namespace,pod) (rate(container_cpu_usage_seconds_total{namespace="${namespace}",pod=~"${workload}.*",container!=""}[5m]))`, `cores`, 0, 6),
		p("workload-memory", "observability.panel.workload_memory", "time_series", `sum by (namespace,pod) (container_memory_working_set_bytes{namespace="${namespace}",pod=~"${workload}.*",container!=""})`, `bytes`, 1, 6),
		p("workload-restarts", "observability.panel.workload_restarts", "time_series", `sum by (namespace,pod) (rate(kube_pod_container_status_restarts_total{namespace="${namespace}",pod=~"${workload}.*"}[5m]))`, `requests_per_second`, 2, 12),
	}},
}

func clone(d Dashboard) Dashboard {
	d.Variables = append([]Variable(nil), d.Variables...)
	d.Panels = append([]Panel(nil), d.Panels...)
	return d
}
func List() []Dashboard {
	out := make([]Dashboard, len(definitions))
	for i := range definitions {
		out[i] = clone(definitions[i])
	}
	return out
}
func Get(code string) (Dashboard, bool) {
	for _, d := range definitions {
		if d.Code == code {
			return clone(d), true
		}
	}
	return Dashboard{}, false
}

var labelSafe = regexp.MustCompile(`^[A-Za-z0-9_.:-]*$`)

func promEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, `\n`, `\n`).Replace(s)
}
func Render(d Dashboard, values map[string]string) (Dashboard, error) {
	allowed := map[string]bool{}
	for _, v := range d.Variables {
		allowed[v.Name] = true
		x := values[v.Name]
		if v.Required && x == "" {
			return Dashboard{}, fmt.Errorf("variable %s is required", v.Name)
		}
		if !labelSafe.MatchString(x) {
			return Dashboard{}, fmt.Errorf("variable %s is not label-safe", v.Name)
		}
	}
	for k := range values {
		if !allowed[k] {
			return Dashboard{}, fmt.Errorf("unknown variable %s", k)
		}
	}
	out := clone(d)
	for i := range out.Panels {
		q := out.Panels[i].PromQL
		for _, v := range d.Variables {
			q = strings.ReplaceAll(q, "${"+v.Name+"}", promEscape(values[v.Name]))
		}
		out.Panels[i].PromQL = q
	}
	return out, nil
}

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }
func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("observability:metric:read"))
	read.GET("/builtin-dashboards", h.List)
	read.GET("/builtin-dashboards/:code", h.Get)
}

// List godoc
// @Summary List built-in metric dashboards
// @Tags observability
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=[]Dashboard}
// @Router /observability/builtin-dashboards [get]
func (h *Handler) List(c *gin.Context) { response.Success(c, List()) }

// Get godoc
// @Summary Get a built-in metric dashboard
// @Tags observability
// @Security BearerAuth
// @Param code path string true "built-in dashboard code"
// @Success 200 {object} response.Envelope{data=Dashboard}
// @Router /observability/builtin-dashboards/{code} [get]
func (h *Handler) Get(c *gin.Context) {
	d, ok := Get(c.Param("code"))
	if !ok {
		response.Error(c, apperr.New(apperr.CodeObservabilityDashboardBuiltinNotFound, "observability.dashboard.builtin_not_found", "built-in dashboard not found"))
		return
	}
	response.Success(c, d)
}
func init() {
	sort.SliceStable(definitions, func(i, j int) bool { return definitions[i].Code < definitions[j].Code })
}
