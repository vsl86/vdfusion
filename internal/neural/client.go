// Package neural provides an HTTP client for the vdfusion neural backend.
// The backend exposes CLIP ViT-B/32 ONNX embeddings over a simple REST API.
package neural

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Client talks to the remote neural embedding backend.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Client pointing at baseURL (e.g. "http://192.168.1.10:8765").
// A 30-second request timeout is applied by default.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HealthCheck returns true if the backend is reachable and its model is loaded.
func (c *Client) HealthCheck(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Info fetches metadata about the running model.
func (c *Client) Info(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/info", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// embedResponse is the JSON shape returned by POST /embed.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed sends a batch of JPEG images to the backend and returns one float32
// embedding vector per image (L2-normalised, 512-dimensional for CLIP ViT-B/32).
//
// images must be raw JPEG/PNG bytes. The order of returned embeddings matches
// the order of the input slice. Returns an error if any image fails or the
// network call fails.
func (c *Client) Embed(ctx context.Context, images [][]byte) ([][]float32, error) {
	if len(images) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for i, img := range images {
		fw, err := mw.CreateFormFile("images", fmt.Sprintf("frame%d.jpg", i))
		if err != nil {
			return nil, fmt.Errorf("neural: create form field: %w", err)
		}
		if _, err := fw.Write(img); err != nil {
			return nil, fmt.Errorf("neural: write image %d: %w", i, err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("neural: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embed", &buf)
	if err != nil {
		return nil, fmt.Errorf("neural: build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("neural: POST /embed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("neural: backend returned %d: %s", resp.StatusCode, body)
	}

	var result embedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("neural: decode response: %w", err)
	}
	if len(result.Embeddings) != len(images) {
		return nil, fmt.Errorf("neural: expected %d embeddings, got %d", len(images), len(result.Embeddings))
	}
	return result.Embeddings, nil
}

// batchRequest represents an internal request to the BatchEmbedder.
type batchRequest struct {
	images     [][]byte
	resultChan chan [][]float32
	errChan    chan error
}

// BatchEmbedder collects multiple small image batches into larger chunks 
// to maximize throughput on the neural backend.
type BatchEmbedder struct {
	client      *Client
	requestChan chan batchRequest
}

// NewBatchEmbedder creates a new BatchEmbedder using the provided client.
func NewBatchEmbedder(client *Client) *BatchEmbedder {
	return &BatchEmbedder{
		client:      client,
		requestChan: make(chan batchRequest),
	}
}

// Start begins the background processing loop for the BatchEmbedder.
func (be *BatchEmbedder) Start(ctx context.Context) {
	go be.run(ctx)
}

// Embed submits a batch of images to the BatchEmbedder and waits for results.
func (be *BatchEmbedder) Embed(ctx context.Context, images [][]byte) ([][]float32, error) {
	if len(images) == 0 {
		return nil, nil
	}

	resChan := make(chan [][]float32, 1)
	errChan := make(chan error, 1)

	req := batchRequest{
		images:     images,
		resultChan: resChan,
		errChan:    errChan,
	}

	// Submit to the aggregator
	select {
	case be.requestChan <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wait for result or context cancellation
	select {
	case res := <-resChan:
		return res, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (be *BatchEmbedder) run(ctx context.Context) {
	var currentBatch [][]byte
	var batchRequests []batchRequest

	// Linger timeout to prevent holding requests too long when throughput is low.
	const linger = 20 * time.Millisecond
	timer := time.NewTimer(linger)
	defer timer.Stop()

	for {
		select {
		case req, ok := <-be.requestChan:
			if !ok {
				// Channel closed, process remaining and exit
				be.process(ctx, currentBatch, batchRequests)
				return
			}

			currentBatch = append(currentBatch, req.images...)
			batchRequests = append(batchRequests, req)

			// If we hit the max batch size (32), process immediately.
			if len(currentBatch) >= 32 {
				be.process(ctx, currentBatch, batchRequests)
				// Reset timer
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(linger)
			}

		case <-timer.C:
			// Linger timeout reached, process current collection.
			if len(currentBatch) > 0 {
				be.process(ctx, currentBatch, batchRequests)
			}
			timer.Reset(linger)

		case <-ctx.Done():
			if len(currentBatch) > 0 {
				be.process(ctx, currentBatch, batchRequests)
			}
			return
		}
	}
}

func (be *BatchEmbedder) process(ctx context.Context, currentBatch [][]byte, batchRequests []batchRequest) {
	if len(currentBatch) == 0 {
		return
	}

	// Perform the actual batch inference via the client.
	allEmbeddings, err := be.client.Embed(ctx, currentBatch)

	// If an error occurs, distribute it to all requests in this batch.
	if err != nil {
		for _, req := range batchRequests {
			req.errChan <- err
		}
		// Clear state for next batch.
		currentBatch = nil
		batchRequests = nil
		return
	}

	// Distribute the flat list of embeddings back to the original requests.
	idx := 0
	for _, req := range batchRequests {
		numInReq := len(req.images)
		if numInReq == 0 {
			continue
		}

		// Slice the results for this specific request.
		res := make([][]float32, numInReq)
		copy(res, allEmbeddings[idx:idx+numInReq])
		req.resultChan <- res
		idx += numInReq
	}

	// Reset state for next batch.
	currentBatch = nil
	batchRequests = nil
}
