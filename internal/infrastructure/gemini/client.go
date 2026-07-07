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

	// Inisialisasi menggunakan format SDK terbaru google.golang.org/genai
	config := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}

	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return nil, err
	}

	// Create a semaphore to strictly limit concurrent requests to Google's API.
	sem := semaphore.NewWeighted(maxConcurrent)

	return &Client{
		client:    client,
		modelName: modelName, // Menyimpan nama model (misal: "gemini-2.5-pro-001")
		sem:       sem,
	}, nil
}

// GenerateSummary calls the Gemini API to analyze the essay.
// It implements the AIGeneratorService interface required by the Application layer.
func (c *Client) GenerateSummary(ctx context.Context, essayText string, locale string) (string, int, error) {
	// PHASE 1: ACQUIRE SEMAPHORE (Waiting in line)
	if err := c.sem.Acquire(ctx, 1); err != nil {
		return "", 0, errors.New("request aborted while waiting for AI slot or context canceled")
	}
	defer c.sem.Release(1)

	// PHASE 2: IN-FLIGHT EXECUTION (Calling the API)
	aiCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	// Security Rule: Role Separation (System vs User) & Locale Enforcement
	sysInstruction := fmt.Sprintf(
		"You are an expert psychologist. Analyze the following essay. "+
			"Respond strictly in the '%s' language. "+
			"Focus on GRIT and MBTI traits. Do not provide clinical diagnosis.",
		locale,
	)

	// FIX 1: Konfigurasi Generation Config pada SDK Baru
	// Menggunakan []*genai.Part dan struct initialization {Text: ...}
	genConfig := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: sysInstruction},
			},
		},
	}

	// Membungkus esai user ke dalam format Content yang benar untuk SDK baru
	userContents := []*genai.Content{
		{
			Parts: []*genai.Part{
				{Text: essayText},
			},
		},
	}
	// Panggilan GenerateContent menggunakan SDK Baru
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

	// FIX 2: Ekstrak teks balasan AI
	// Tidak perlu type assertion (part.(genai.Text)) lagi,
	// karena 'part' sudah berupa struct pointer (*genai.Part) yang memiliki properti Text.
	var summary string
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			summary += part.Text
		}
	}

	// Ekstrak Token Usage untuk Cost Tracking
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
