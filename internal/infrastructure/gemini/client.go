package gemini

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	// Menggunakan SDK Resmi terbaru dari Google (2025+)
	"golang.org/x/sync/semaphore"
	"google.golang.org/genai"
)

// Client represents the Gemini API infrastructure implementation using the new SDK.
type Client struct {
	client    *genai.Client
	modelName string
	sem       *semaphore.Weighted
}

// NewClient initializes the Gemini client with strict model pinning and concurrency limits.
func NewClient(apiKey string, modelName string, maxConcurrent int64) (*Client, error) {
	ctx := context.Background()

	var client *genai.Client
	var err error

	if apiKey != "" {
		config := &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		}
		client, err = genai.NewClient(ctx, config)
		if err != nil {
			return nil, err
		}
	} else {
		log.Println("[Gemini] WARNING: GEMINI_API_KEY is not set. AI assessment features will be unavailable.")
	}

	sem := semaphore.NewWeighted(maxConcurrent)

	return &Client{
		client:    client,
		modelName: modelName,
		sem:       sem,
	}, nil
}

// GenerateSummary calls the Gemini API to analyze the essay.
// It implements the AIGeneratorService interface required by the Application layer.
func (c *Client) GenerateSummary(ctx context.Context, essayText string, locale string) (string, int, error) {
	if c.client == nil {
		return "", 0, errors.New("gemini client is unconfigured: GEMINI_API_KEY environment variable is empty")
	}

	if err := c.sem.Acquire(ctx, 1); err != nil {
		return "", 0, errors.New("request aborted while waiting for AI slot or context canceled")
	}
	defer c.sem.Release(1)

	aiCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	// Security Rule: Role Separation (System vs User) & Locale Enforcement
	sysInstruction := fmt.Sprintf(
		"You are an expert psychologist. Analyze the following essay. "+
			"Respond strictly in the '%s' language. "+
			"Focus on GRIT and MBTI traits. Do not provide clinical diagnosis.",
		locale,
	)

	genConfig := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: sysInstruction},
			},
		},
	}

	userContents := []*genai.Content{
		{
			Parts: []*genai.Part{
				{Text: essayText},
			},
		},
	}
	resp, err := c.client.Models.GenerateContent(
		aiCtx,
		c.modelName,
		userContents,
		genConfig,
	)

	if err != nil {
		log.Printf("[Gemini API] Failed to generate content: %v\n", err)
		return "", 0, err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", 0, errors.New("empty response from Gemini")
	}

	var summary string
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			summary += part.Text
		}
	}

	totalTokens := 0
	if resp.UsageMetadata != nil {
		totalTokens = int(resp.UsageMetadata.TotalTokenCount)
	}

	return summary, totalTokens, nil
}

// Close gracefully shuts down the Gemini client if needed (SDK baru menangani resource secara efisien).
func (c *Client) Close() {
	// Pada SDK baru genai.Client tidak memiliki metode Close() eksplisit,
	// karena koneksi dikelola via HTTP transport. Method ini bisa dibiarkan kosong
	// atau digunakan untuk menutup antrean internal jika ada di masa depan.
}
