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

const (
	DefaultUrl          = "http://localhost:8181"
	DefaultDatabase     = "benchmarks"
	DefaultToken        = "benchmark-token"
	DefaultSampleRate   = 0.1
	defaultWriteTimeout = 15 * time.Second
)

func waitToBeReady(ctx context.Context) error {
	healthUrl := DefaultUrl + "/health"
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthUrl, http.NoBody)
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
			cli.Warnf("InfluxDB not available at %s, metrics export disabled", DefaultUrl)
			return errors.New("influxdb not available, metrics export disabled")
		}

		time.Sleep(2 * time.Second)
	}
	return nil
}

//nolint:contextcheck // context is stored in Client for use in async write operations
func NewClient(ctx context.Context, sampleRate float64) *Client {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := waitToBeReady(ctx); err != nil {
		return nil
	}

	client, err := influxdb3.New(influxdb3.ClientConfig{
		Host:     DefaultUrl,
		Token:    DefaultToken,
		Database: DefaultDatabase,
	})
	if err != nil {
		cli.Warnf("Failed to create InfluxDB client: %v", err)
		return nil
	}

	if sampleRate <= 0 || sampleRate > 1 {
		sampleRate = DefaultSampleRate
	}

	return &Client{
		client:     client,
		database:   DefaultDatabase,
		ctx:        ctx,
		timeout:    defaultWriteTimeout,
		sampleRate: sampleRate,
	}
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	if err := c.client.Close(); err != nil {
		cli.Warnf("Failed to close InfluxDB client: %v", err)
	}
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

	if c.ctx.Err() != nil {
		return
	}

	ctx := c.ctx
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
		return
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

func RunId(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}
