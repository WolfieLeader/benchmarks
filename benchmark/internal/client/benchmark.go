package client

func (c *Client) Benchmark() (*Stats, error) {
	endpoint := &Endpoint{
		Path:   "/",
		Method: "GET",
		Testcases: []*Testcase{
			{StatusCode: 200, Body: map[string]any{"message": "Hello, World!"}},
		},
	}

	return c.RunEndpointN(endpoint, 1000, 50)
}
