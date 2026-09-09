package openai

import "github.com/twistingmercury/mnemonic-api/internal/config"

// NewEmbeddingServiceForTest creates an EmbeddingService pointing at a custom URL.
// Exported for use in black-box tests.
var NewEmbeddingServiceForTest = newEmbeddingServiceWithURL

// newEmbeddingServiceWithURL creates an EmbeddingService pointing at a custom URL.
// This is used for testing with httptest servers.
func newEmbeddingServiceWithURL(cfg config.OpenAIConfig, baseURL string) EmbeddingService {
	svc := NewEmbeddingService(cfg).(*openaiEmbedding)
	svc.baseURL = baseURL
	return svc
}
