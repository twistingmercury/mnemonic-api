package openai

// NewEmbeddingServiceForTest creates an EmbeddingService pointing at a custom URL.
// Exported for use in black-box tests.
var NewEmbeddingServiceForTest = newEmbeddingServiceWithURL
