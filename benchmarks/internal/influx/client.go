package influx

import (
	"context"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"benchmark-client/internal/printer"
)

// Client wraps InfluxDB write operations.
type Client struct {
	client   influxdb2.Client
	writeAPI api.WriteAPI
	org      string
	bucket   string
}

// NewClient creates a new InfluxDB client.
// Returns nil if connection fails (graceful degradation).
func NewClient(cfg Config) *Client {
	if !cfg.Enabled {
		return nil
	}

	client := influxdb2.NewClient(cfg.URL, cfg.Token)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil || health.Status != "pass" {
		printer.Warnf("InfluxDB not available at %s, metrics export disabled", cfg.URL)
		client.Close()
		return nil
	}

	writeAPI := client.WriteAPI(cfg.Org, cfg.Bucket)

	// Handle write errors
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

// Close flushes pending writes and closes the connection.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.writeAPI.Flush()
	c.client.Close()
}

// WritePoint writes a single point to InfluxDB.
func (c *Client) WritePoint(measurement string, tags map[string]string, fields map[string]any, ts time.Time) {
	if c == nil {
		return
	}
	p := influxdb2.NewPoint(measurement, tags, fields, ts)
	c.writeAPI.WritePoint(p)
}

// Flush forces all pending writes to be sent.
func (c *Client) Flush() {
	if c == nil {
		return
	}
	c.writeAPI.Flush()
}

// RunID generates a unique run identifier from timestamp.
func RunID(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}
