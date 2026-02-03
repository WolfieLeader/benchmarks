package influx

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/influxdb3"

	"benchmark-client/internal/cli"
)

type Client struct {
	client     *influxdb3.Client
	database   string
	ctx        context.Context
	timeout    time.Duration
	sampleRate float64
	wg         sync.WaitGroup
}

type Config struct {
	URL        string  `json:"url"`
	Database   string  `json:"database"`
	Token      string  `json:"token"`
	SampleRate float64 `json:"sample_rate"`
}

const defaultWriteTimeout = 15 * time.Second

//nolint:contextcheck // context is stored in Client for use in async write operations
func NewClient(ctx context.Context, cfg Config) *Client {
	if ctx == nil {
		ctx = context.Background()
	}

	// Check if InfluxDB is healthy via HTTP before creating client
	healthURL := cfg.URL + "/health"
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return nil
		}

		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthURL, http.NoBody)
		if err != nil {
			cancel()
			time.Sleep(2 * time.Second)
			continue
		}

		resp, err := http.DefaultClient.Do(req)
		cancel()

		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}

		if time.Now().Add(2 * time.Second).After(deadline) {
			cli.Warnf("InfluxDB not available at %s, metrics export disabled", cfg.URL)
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	client, err := influxdb3.New(influxdb3.ClientConfig{
		Host:     cfg.URL,
		Token:    cfg.Token,
		Database: cfg.Database,
	})
	if err != nil {
		cli.Warnf("Failed to create InfluxDB client: %v", err)
		return nil
	}

	sampleRate := cfg.SampleRate
	if sampleRate <= 0 || sampleRate > 1 {
		sampleRate = 0.1
	}

	return &Client{
		client:     client,
		database:   cfg.Database,
		ctx:        ctx,
		timeout:    defaultWriteTimeout,
		sampleRate: sampleRate,
	}
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	_ = c.client.Close()
}

func (c *Client) WritePoint(measurement string, tags map[string]string, fields map[string]any, ts time.Time) {
	if c == nil {
		return
	}

	p := influxdb3.NewPoint(measurement, tags, fields, ts)
	c.writePoints([]*influxdb3.Point{p})
}

func (c *Client) writePoints(points []*influxdb3.Point) {
	if c == nil || len(points) == 0 {
		return
	}

	if c.ctx != nil && c.ctx.Err() != nil {
		return
	}

	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	if err := c.client.WritePoints(ctx, points); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		cli.Warnf("InfluxDB write error (%d points): %v", len(points), err)
	}
}

func (c *Client) writePointsAsync(points []*influxdb3.Point) {
	if c == nil || len(points) == 0 {
		return
	}

	pointsCopy := make([]*influxdb3.Point, len(points))
	copy(pointsCopy, points)

	c.wg.Go(func() {
		c.writePoints(pointsCopy)
	})
}

func (c *Client) Wait() {
	if c == nil {
		return
	}
	c.wg.Wait()
}

func RunID(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}
