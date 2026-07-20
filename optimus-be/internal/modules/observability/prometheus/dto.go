package prometheus

type Sample struct {
	Timestamp float64 `json:"timestamp"`
	Value     string  `json:"value"`
}

type Series struct {
	Labels  map[string]string `json:"labels"`
	Samples []Sample          `json:"samples"`
}

type Result struct {
	ResultType string   `json:"result_type"`
	Series     []Series `json:"series,omitempty"`
	Scalar     *Sample  `json:"scalar,omitempty"`
	Text       *string  `json:"text,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}
