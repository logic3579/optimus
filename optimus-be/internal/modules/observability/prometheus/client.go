package prometheus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var labelNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Client struct {
	http      *http.Client
	base      *url.URL
	maxBody   int64
	maxSeries int
}

func NewClient(httpClient *http.Client, base *url.URL, maxBody int64, maxSeries int) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	copyBase := new(url.URL)
	if base != nil {
		*copyBase = *base
	}
	return &Client{http: httpClient, base: copyBase, maxBody: maxBody, maxSeries: maxSeries}
}

func (c *Client) Query(ctx context.Context, promql string, at time.Time) (Result, error) {
	v := url.Values{"query": {promql}}
	if !at.IsZero() {
		v.Set("time", formatTime(at))
	}
	return c.query(ctx, "api/v1/query", v)
}
func (c *Client) QueryRange(ctx context.Context, promql string, start, end time.Time, step time.Duration) (Result, error) {
	if step <= 0 || end.Before(start) {
		return Result{}, invalidRequest(errors.New("invalid query range"))
	}
	v := url.Values{"query": {promql}, "start": {formatTime(start)}, "end": {formatTime(end)}, "step": {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)}}
	return c.query(ctx, "api/v1/query_range", v)
}
func (c *Client) Labels(ctx context.Context) ([]string, error) {
	var out []string
	err := c.request(ctx, http.MethodGet, "api/v1/labels", nil, &out)
	return out, err
}
func (c *Client) LabelValues(ctx context.Context, label string) ([]string, error) {
	if !labelNamePattern.MatchString(label) {
		return nil, invalidRequest(errors.New("invalid label name"))
	}
	var out []string
	err := c.request(ctx, http.MethodGet, "api/v1/label/"+url.PathEscape(label)+"/values", nil, &out)
	return out, err
}
func (c *Client) BuildInfo(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	err := c.request(ctx, http.MethodGet, "api/v1/status/buildinfo", nil, &out)
	return out, err
}

func (c *Client) query(ctx context.Context, path string, form url.Values) (Result, error) {
	var raw queryData
	var env responseEnvelope
	if err := c.requestEnvelope(ctx, http.MethodPost, path, form, &env); err != nil {
		return Result{}, err
	}
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		return Result{}, invalidResponse(err)
	}
	return normalize(raw, env.Warnings, c.maxSeries)
}

func (c *Client) request(ctx context.Context, method, path string, form url.Values, out any) error {
	var env responseEnvelope
	if err := c.requestEnvelope(ctx, method, path, form, &env); err != nil {
		return err
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return invalidResponse(err)
	}
	return nil
}

type responseEnvelope struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings"`
}

func (c *Client) requestEnvelope(ctx context.Context, method, path string, form url.Values, out *responseEnvelope) error {
	endpoint, err := c.endpoint(path)
	if err != nil {
		return invalidRequest(err)
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return invalidRequest(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return mapClientError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode >= 500 {
		_, readErr := readBounded(resp.Body, c.maxBody)
		if readErr != nil {
			return invalidResponse(readErr)
		}
		return rejected(fmt.Errorf("Prometheus returned HTTP %d", resp.StatusCode))
	}
	data, err := readBounded(resp.Body, c.maxBody)
	if err != nil {
		return invalidResponse(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return rejected(fmt.Errorf("Prometheus returned HTTP %d", resp.StatusCode))
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(out); err != nil {
		return invalidResponse(err)
	}
	if out.Status != "success" {
		return rejected(fmt.Errorf("Prometheus API error type %q", out.ErrorType))
	}
	if len(out.Data) == 0 || bytes.Equal(out.Data, []byte("null")) {
		return invalidResponse(errors.New("missing Prometheus data"))
	}
	return nil
}
func (c *Client) endpoint(path string) (*url.URL, error) {
	if c == nil || c.base == nil || c.base.Scheme == "" || c.base.Host == "" {
		return nil, errors.New("invalid Prometheus client base URL")
	}
	u := *c.base
	prefix := strings.TrimSuffix(u.Path, "/")
	u.Path = prefix + "/" + strings.TrimPrefix(path, "/")
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return &u, nil
}
func readBounded(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		return nil, errors.New("invalid response size limit")
	}
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("Prometheus response exceeds size limit")
	}
	return data, nil
}
func formatTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', -1, 64)
}

type queryData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}
type rawSeries struct {
	Metric map[string]string `json:"metric"`
	Value  json.RawMessage   `json:"value"`
	Values []json.RawMessage `json:"values"`
}

func normalize(raw queryData, warnings []string, maxSeries int) (Result, error) {
	result := Result{ResultType: raw.ResultType, Warnings: warnings}
	switch raw.ResultType {
	case "vector", "matrix":
		var rows []rawSeries
		if err := json.Unmarshal(raw.Result, &rows); err != nil {
			return Result{}, invalidResponse(err)
		}
		if maxSeries <= 0 || len(rows) > maxSeries {
			return Result{}, invalidResponse(errors.New("series limit exceeded"))
		}
		result.Series = make([]Series, 0, len(rows))
		for _, row := range rows {
			if err := validateLabels(row.Metric); err != nil {
				return Result{}, invalidResponse(err)
			}
			series := Series{Labels: row.Metric}
			if raw.ResultType == "vector" {
				sample, err := decodeSample(row.Value)
				if err != nil {
					return Result{}, invalidResponse(err)
				}
				series.Samples = []Sample{sample}
			} else {
				series.Samples = make([]Sample, 0, len(row.Values))
				for _, tuple := range row.Values {
					sample, err := decodeSample(tuple)
					if err != nil {
						return Result{}, invalidResponse(err)
					}
					series.Samples = append(series.Samples, sample)
				}
			}
			result.Series = append(result.Series, series)
		}
	case "scalar", "string":
		sample, err := decodeSample(raw.Result)
		if err != nil {
			return Result{}, invalidResponse(err)
		}
		if raw.ResultType == "scalar" {
			result.Scalar = &sample
		} else {
			result.Text = &sample.Value
		}
	default:
		return Result{}, invalidResponse(errors.New("unsupported Prometheus result type"))
	}
	return result, nil
}
func decodeSample(raw json.RawMessage) (Sample, error) {
	var tuple []json.RawMessage
	if err := json.Unmarshal(raw, &tuple); err != nil || len(tuple) != 2 {
		return Sample{}, errors.New("invalid sample tuple")
	}
	var ts float64
	if err := json.Unmarshal(tuple[0], &ts); err != nil || math.IsNaN(ts) || math.IsInf(ts, 0) {
		return Sample{}, errors.New("invalid sample timestamp")
	}
	var value string
	if err := json.Unmarshal(tuple[1], &value); err != nil {
		return Sample{}, errors.New("invalid sample value")
	}
	return Sample{Timestamp: ts, Value: value}, nil
}
func validateLabels(labels map[string]string) error {
	if len(labels) > 128 {
		return errors.New("too many labels")
	}
	for key, value := range labels {
		if len(key) > 256 || len(value) > 4096 {
			return errors.New("label exceeds limit")
		}
	}
	return nil
}
