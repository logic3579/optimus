package prometheus

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	apperr "optimus-be/internal/infra/errors"

	"github.com/stretchr/testify/require"
)

func TestClientQueryRangeMatrixAndFormEncoding(t *testing.T) {
	var seenPath, seenQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath, seenQuery = r.URL.Path, r.URL.RawQuery
		require.NoError(t, r.ParseForm())
		require.Equal(t, "rate(http_requests_total[5m]) + x&y", r.Form.Get("query"))
		io.WriteString(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"pod":"api-0"},"values":[[1,"2.5"],[2,"3.5"]]}]}}`)
	}))
	defer server.Close()
	base := mustClientURL(t, server.URL+"/prometheus")
	c := NewClient(server.Client(), base, 1<<20, 1000)
	got, err := c.QueryRange(context.Background(), "rate(http_requests_total[5m]) + x&y", time.Unix(1, 0), time.Unix(2, 0), time.Second)
	require.NoError(t, err)
	require.Equal(t, "/prometheus/api/v1/query_range", seenPath)
	require.NotContains(t, seenQuery, "http_requests_total[5m]")
	require.Equal(t, "matrix", got.ResultType)
	require.Equal(t, "api-0", got.Series[0].Labels["pod"])
	require.Equal(t, []Sample{{Timestamp: 1, Value: "2.5"}, {Timestamp: 2, Value: "3.5"}}, got.Series[0].Samples)
}

func TestClientNormalizesResultTypesAndWarnings(t *testing.T) {
	tests := []struct {
		name, body string
		check      func(*testing.T, Result)
	}{
		{"vector", `{"status":"success","warnings":["partial"],"data":{"resultType":"vector","result":[{"metric":{"job":"api"},"value":[1,"2"]}]}}`, func(t *testing.T, r Result) {
			require.Equal(t, "partial", r.Warnings[0])
			require.Equal(t, "2", r.Series[0].Samples[0].Value)
		}},
		{"scalar", `{"status":"success","data":{"resultType":"scalar","result":[1,"2"]}}`, func(t *testing.T, r Result) { require.Equal(t, "2", r.Scalar.Value) }},
		{"string", `{"status":"success","data":{"resultType":"string","result":[1,"up"]}}`, func(t *testing.T, r Result) { require.Equal(t, "up", *r.Text) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := prometheusJSONServer(t, http.StatusOK, tc.body)
			defer s.Close()
			got, err := NewClient(s.Client(), mustClientURL(t, s.URL), 1<<20, 10).Query(context.Background(), "up", time.Unix(1, 0))
			require.NoError(t, err)
			require.Equal(t, tc.name, got.ResultType)
			tc.check(t, got)
		})
	}
}

func TestClientMetadataEndpointsAndFixedPrefix(t *testing.T) {
	responses := map[string]string{
		"/fixed/api/v1/labels":                `{"status":"success","data":["job","pod"]}`,
		"/fixed/api/v1/label/pod_name/values": `{"status":"success","data":["a","b"]}`,
		"/fixed/api/v1/status/buildinfo":      `{"status":"success","data":{"version":"2.50","revision":"abc"}}`,
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.Path]
		require.True(t, ok)
		io.WriteString(w, body)
	}))
	defer s.Close()
	c := NewClient(s.Client(), mustClientURL(t, s.URL+"/fixed/"), 1<<20, 10)
	labels, err := c.Labels(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"job", "pod"}, labels)
	values, err := c.LabelValues(context.Background(), "pod_name")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, values)
	info, err := c.BuildInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, "2.50", info["version"])
	_, err = c.LabelValues(context.Background(), "../bad")
	requireBizCode(t, err, apperr.CodeObservabilityQueryInvalidRequest)
}

func TestClientMapsFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		code   apperr.Code
	}{
		{"api error", 200, `{"status":"error","errorType":"bad_data","error":"secret upstream detail"}`, apperr.CodeObservabilityQueryUpstreamRejected},
		{"malformed", 200, `{`, apperr.CodeObservabilityQueryInvalidResponse},
		{"unauthorized", 401, `no`, apperr.CodeObservabilityQueryUpstreamRejected},
		{"forbidden", 403, `no`, apperr.CodeObservabilityQueryUpstreamRejected},
		{"server", 503, `no`, apperr.CodeObservabilityQueryUpstreamRejected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := prometheusJSONServer(t, tc.status, tc.body)
			defer s.Close()
			_, err := NewClient(s.Client(), mustClientURL(t, s.URL), 1<<20, 10).Query(context.Background(), "up", time.Time{})
			be := requireClientBizCode(t, err, tc.code)
			require.NotContains(t, be.Message, "secret upstream detail")
			require.Error(t, be.Cause)
		})
	}
	t.Run("oversize", func(t *testing.T) {
		s := prometheusJSONServer(t, 200, strings.Repeat("x", 65))
		defer s.Close()
		_, err := NewClient(s.Client(), mustClientURL(t, s.URL), 64, 10).Query(context.Background(), "up", time.Time{})
		requireBizCode(t, err, apperr.CodeObservabilityQueryInvalidResponse)
	})
	t.Run("timeout", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(250 * time.Millisecond):
			}
		}))
		defer s.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := NewClient(s.Client(), mustClientURL(t, s.URL), 1024, 10).Query(ctx, "up", time.Time{})
		be := requireClientBizCode(t, err, apperr.CodeObservabilityQueryUpstreamTimeout)
		require.ErrorIs(t, be.Cause, context.DeadlineExceeded)
	})
}

func TestClientRejectsTrailingJSONContent(t *testing.T) {
	valid := `{"status":"success","data":{"resultType":"vector","result":[]}}`
	for _, tc := range []struct {
		name   string
		suffix string
	}{
		{"trailing garbage", ` garbage`},
		{"second JSON object", ` {"status":"success","data":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := prometheusJSONServer(t, http.StatusOK, valid+tc.suffix)
			defer s.Close()
			_, err := NewClient(s.Client(), mustClientURL(t, s.URL), 1<<20, 10).Query(context.Background(), "up", time.Time{})
			requireBizCode(t, err, apperr.CodeObservabilityQueryInvalidResponse)
		})
	}
}

func TestClientMapsBodyReadDeadlineToTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := io.WriteString(w, `{"status":"success","data":`)
		require.NoError(t, err)
		require.NoError(t, http.NewResponseController(w).Flush())
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := NewClient(s.Client(), mustClientURL(t, s.URL), 1<<20, 10).Query(ctx, "up", time.Time{})
	be := requireClientBizCode(t, err, apperr.CodeObservabilityQueryUpstreamTimeout)
	require.ErrorIs(t, be.Cause, context.DeadlineExceeded)
}

func TestClientRejectsInvalidNormalizedDTOs(t *testing.T) {
	labels := make([]string, 129)
	for i := range labels {
		labels[i] = `"k` + strings.Repeat("x", i) + `":"v"`
	}
	hugeLabels := "{" + strings.Join(labels, ",") + "}"
	tests := []string{
		`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1]]}]}}`,
		`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":["NaN","1"]}]}}`,
		`{"status":"success","data":{"resultType":"vector","result":[{"metric":` + hugeLabels + `,"value":[1,"1"]}]}}`,
		`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"` + strings.Repeat("k", 257) + `":"v"},"value":[1,"1"]}]}}`,
		`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"k":"` + strings.Repeat("v", 4097) + `"},"value":[1,"1"]}]}}`,
	}
	for i, body := range tests {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			s := prometheusJSONServer(t, 200, body)
			defer s.Close()
			_, err := NewClient(s.Client(), mustClientURL(t, s.URL), 1<<20, 10).Query(context.Background(), "up", time.Time{})
			requireBizCode(t, err, apperr.CodeObservabilityQueryInvalidResponse)
		})
	}
	t.Run("series limit", func(t *testing.T) {
		body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"1"]},{"metric":{},"value":[1,"2"]}]}}`
		s := prometheusJSONServer(t, 200, body)
		defer s.Close()
		_, err := NewClient(s.Client(), mustClientURL(t, s.URL), 1<<20, 1).Query(context.Background(), "up", time.Time{})
		requireBizCode(t, err, apperr.CodeObservabilityQueryInvalidResponse)
	})
}

type errorTransport struct{ err error }

func (e errorTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

func TestClientMapsTransportErrorsAndPreservesCause(t *testing.T) {
	destination := deniedDestination()
	for _, tc := range []struct {
		name string
		err  error
		code apperr.Code
	}{{"denied", destination, apperr.CodeObservabilityQueryDestinationDenied}, {"dial", errors.New("dial failed"), apperr.CodeObservabilityQueryUpstreamUnreachable}} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(&http.Client{Transport: errorTransport{tc.err}}, mustClientURL(t, "http://example.com/base"), 1024, 10)
			_, err := c.Query(context.Background(), "up", time.Time{})
			be := requireClientBizCode(t, err, tc.code)
			require.ErrorIs(t, be.Cause, tc.err)
		})
	}
}

func prometheusJSONServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, err := io.WriteString(w, body)
		require.NoError(t, err)
	}))
}
func mustClientURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func requireClientBizCode(t *testing.T, err error, code apperr.Code) *apperr.BizError {
	t.Helper()
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, code, be.Code)
	return be
}
