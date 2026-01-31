package influx

type Config struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Org     string `json:"org"`
	Bucket  string `json:"bucket"`
	Token   string `json:"token"`
}

func DefaultConfig() Config {
	return Config{
		Enabled: false,
		URL:     "http://localhost:8086",
		Org:     "benchmarks",
		Bucket:  "benchmarks",
		Token:   "benchmark-token",
	}
}
