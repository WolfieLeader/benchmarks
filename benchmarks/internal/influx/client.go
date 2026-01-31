package influx

import (
	"context"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"benchmark-client/internal/printer"
)

type Client struct {
	client   influxdb2.Client
	writeAPI api.WriteAPI
	org      string
	bucket   string
}

func NewClient(ctx context.Context, cfg Config) *Client {
	if !cfg.Enabled {
		return nil
	}

	client := influxdb2.NewClient(cfg.URL, cfg.Token)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			client.Close()
			return nil
		}

		healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		health, err := client.Health(healthCtx)
		cancel()

		if err == nil && health.Status == "pass" {
			break
		}

		if time.Now().Add(2 * time.Second).After(deadline) {
			printer.Warnf("InfluxDB not available at %s, metrics export disabled", cfg.URL)
			client.Close()
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	writeAPI := client.WriteAPI(cfg.Org, cfg.Bucket)

	go func() {
		for err := range writeAPI.Errors() {
			printer.Warnf("InfluxDB write error: %v", err)
		}
	}()

	printer.Infof("InfluxDB connected: %s", cfg.URL)

	return &Client{
		client:   client,
		writeAPI: writeAPI,
		org:      cfg.Org,
		bucket:   cfg.Bucket,
	}
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.writeAPI.Flush()
	c.client.Close()
}

func (c *Client) WritePoint(measurement string, tags map[string]string, fields map[string]any, ts time.Time) {
	if c == nil {
		return
	}
	p := influxdb2.NewPoint(measurement, tags, fields, ts)
	c.writeAPI.WritePoint(p)
}

func (c *Client) Flush() {
	if c == nil {
		return
	}
	c.writeAPI.Flush()
}

func RunID(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}
