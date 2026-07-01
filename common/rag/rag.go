package rag

import (
	redisPkg "aiplatform/common/redis"
	"aiplatform/config"
	"context"
	"fmt"
	"os"
	"strings"

	embeddingArk "github.com/cloudwego/eino-ext/components/embedding/ark"
	redisIndexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisRetriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	redisCli "github.com/redis/go-redis/v9"
)

type RAGIndexer struct {
	embedding embedding.Embedder
	indexer   *redisIndexer.Indexer
}

type RAGQuery struct {
	embedding embedding.Embedder
	retriever retriever.Retriever
}

const (
	defaultChunkSize    = 800
	defaultChunkOverlap = 150
	defaultTopK         = 5
)

// NewRAGIndexer creates the embedder, Redis vector index, and document indexer.
func NewRAGIndexer(filename, embeddingModel string) (*RAGIndexer, error) {
	ctx := context.Background()
	cfg := config.GetConfig()
	apiKey := os.Getenv("OPENAI_API_KEY")
	dimension := cfg.RagModelConfig.RagDimension

	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: cfg.RagModelConfig.RagBaseUrl,
		APIKey:  apiKey,
		Model:   embeddingModel,
	}

	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	if err := redisPkg.InitRedisIndex(ctx, filename, dimension); err != nil {
		return nil, fmt.Errorf("failed to init redis index: %w", err)
	}

	indexerConfig := &redisIndexer.IndexerConfig{
		Client:    redisPkg.Rdb,
		KeyPrefix: redisPkg.GenerateIndexNamePrefix(filename),
		BatchSize: 10,
		DocumentToHashes: func(ctx context.Context, doc *schema.Document) (*redisIndexer.Hashes, error) {
			source := ""
			if s, ok := doc.MetaData["source"].(string); ok {
				source = s
			}

			return &redisIndexer.Hashes{
				Key: fmt.Sprintf("%s:%s", filename, doc.ID),
				Field2Value: map[string]redisIndexer.FieldValue{
					"content":  {Value: doc.Content, EmbedKey: "vector"},
					"metadata": {Value: source},
				},
			}, nil
		},
	}
	indexerConfig.Embedding = embedder

	idx, err := redisIndexer.NewIndexer(ctx, indexerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create indexer: %w", err)
	}

	return &RAGIndexer{
		embedding: embedder,
		indexer:   idx,
	}, nil
}

// IndexFile reads a file, splits it into overlapping chunks, and stores vectors.
func (r *RAGIndexer) IndexFile(ctx context.Context, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	chunkSize, chunkOverlap := getChunkConfig()
	chunks := splitText(string(content), chunkSize, chunkOverlap)
	if len(chunks) == 0 {
		return fmt.Errorf("file content is empty")
	}

	docs := make([]*schema.Document, 0, len(chunks))
	for i, chunk := range chunks {
		docs = append(docs, &schema.Document{
			ID:      fmt.Sprintf("chunk_%04d", i+1),
			Content: chunk,
			MetaData: map[string]any{
				"source":      filePath,
				"chunk_index": i,
				"chunk_total": len(chunks),
			},
		})
	}

	_, err = r.indexer.Store(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to store document: %w", err)
	}

	return nil
}

func splitText(text string, chunkSize, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	chunkSize, overlap = normalizeChunkConfig(chunkSize, overlap)
	paragraphs := splitParagraphs(text)
	if len(paragraphs) == 0 {
		return nil
	}

	chunks := make([]string, 0, len(paragraphs))
	var current strings.Builder
	currentLen := 0

	flushCurrent := func() {
		chunk := strings.TrimSpace(current.String())
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		current.Reset()
		currentLen = 0
	}

	for _, paragraph := range paragraphs {
		paragraphLen := len([]rune(paragraph))
		if paragraphLen > chunkSize {
			flushCurrent()
			chunks = append(chunks, splitByRuneWindow(paragraph, chunkSize, overlap)...)
			continue
		}

		separatorLen := 0
		if currentLen > 0 {
			separatorLen = 2
		}

		if currentLen > 0 && currentLen+separatorLen+paragraphLen > chunkSize {
			previous := strings.TrimSpace(current.String())
			flushCurrent()

			prefix := suffixByRunes(previous, overlap)
			if prefix != "" && len([]rune(prefix))+2+paragraphLen <= chunkSize {
				current.WriteString(prefix)
				current.WriteString("\n\n")
				currentLen = len([]rune(prefix)) + 2
			}
		}

		if currentLen > 0 {
			current.WriteString("\n\n")
			currentLen += 2
		}
		current.WriteString(paragraph)
		currentLen += paragraphLen
	}

	flushCurrent()
	return chunks
}

func splitByRuneWindow(text string, chunkSize, overlap int) []string {
	chunkSize, overlap = normalizeChunkConfig(chunkSize, overlap)

	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= chunkSize {
		return []string{strings.TrimSpace(text)}
	}

	step := chunkSize - overlap
	chunks := make([]string, 0, len(runes)/step+1)
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
	}

	return chunks
}

func splitParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")
	paragraphs := make([]string, 0)
	var current strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			paragraph := strings.TrimSpace(current.String())
			if paragraph != "" {
				paragraphs = append(paragraphs, paragraph)
				current.Reset()
			}
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	paragraph := strings.TrimSpace(current.String())
	if paragraph != "" {
		paragraphs = append(paragraphs, paragraph)
	}

	return paragraphs
}

func suffixByRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}

	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}

	return strings.TrimSpace(string(runes[len(runes)-limit:]))
}

func getChunkConfig() (int, int) {
	cfg := config.GetConfig().RagModelConfig
	return normalizeChunkConfig(cfg.RagChunkSize, cfg.RagChunkOverlap)
}

func normalizeChunkConfig(chunkSize, overlap int) (int, int) {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}
	return chunkSize, overlap
}

func getTopKConfig() int {
	topK := config.GetConfig().RagModelConfig.RagTopK
	if topK <= 0 {
		return defaultTopK
	}
	return topK
}

// DeleteIndex removes the Redis vector index for the uploaded file.
func DeleteIndex(ctx context.Context, filename string) error {
	if err := redisPkg.DeleteRedisIndex(ctx, filename); err != nil {
		return fmt.Errorf("failed to delete redis index: %w", err)
	}
	return nil
}

// NewRAGQuery creates a retriever for the current user's uploaded file.
func NewRAGQuery(ctx context.Context, username string) (*RAGQuery, error) {
	cfg := config.GetConfig()
	apiKey := os.Getenv("OPENAI_API_KEY")

	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: cfg.RagModelConfig.RagBaseUrl,
		APIKey:  apiKey,
		Model:   cfg.RagModelConfig.RagEmbeddingModel,
	}
	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	userDir := fmt.Sprintf("uploads/%s", username)
	files, err := os.ReadDir(userDir)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no uploaded file found for user %s", username)
	}

	var filename string
	for _, f := range files {
		if !f.IsDir() {
			filename = f.Name()
			break
		}
	}

	if filename == "" {
		return nil, fmt.Errorf("no valid file found for user %s", username)
	}

	retrieverConfig := &redisRetriever.RetrieverConfig{
		Client:       redisPkg.Rdb,
		Index:        redisPkg.GenerateIndexName(filename),
		Dialect:      2,
		ReturnFields: []string{"content", "metadata", "distance"},
		TopK:         getTopKConfig(),
		VectorField:  "vector",
		DocumentConverter: func(ctx context.Context, doc redisCli.Document) (*schema.Document, error) {
			resp := &schema.Document{
				ID:       doc.ID,
				Content:  "",
				MetaData: map[string]any{},
			}
			for field, val := range doc.Fields {
				if field == "content" {
					resp.Content = val
				} else {
					resp.MetaData[field] = val
				}
			}
			return resp, nil
		},
	}
	retrieverConfig.Embedding = embedder

	rtr, err := redisRetriever.NewRetriever(ctx, retrieverConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create retriever: %w", err)
	}

	return &RAGQuery{
		embedding: embedder,
		retriever: rtr,
	}, nil
}

// RetrieveDocuments retrieves semantically relevant chunks.
func (r *RAGQuery) RetrieveDocuments(ctx context.Context, query string) ([]*schema.Document, error) {
	docs, err := r.retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve documents: %w", err)
	}
	return docs, nil
}

// BuildRAGPrompt builds the prompt with retrieved document chunks.
func BuildRAGPrompt(query string, docs []*schema.Document) string {
	if len(docs) == 0 {
		return query
	}

	var contextText strings.Builder
	for i, doc := range docs {
		contextText.WriteString(fmt.Sprintf("[Document %d]: %s\n\n", i+1, doc.Content))
	}

	return fmt.Sprintf(`Answer the user's question based on the reference documents below. If the documents do not contain relevant information, say that no relevant information was found.

Reference documents:
%s

User question:
%s

Please provide an accurate and complete answer.`, contextText.String(), query)
}
