package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gis-backend/config"
)

const embeddingSize = 384

type modelClient struct {
	base string
	http *http.Client
}

func newModelClient() modelClient {
	return modelClient{
		base: config.AssistantURL(),
		http: &http.Client{Timeout: 15 * time.Minute},
	}
}

type preparedChunk struct {
	Number int       `json:"number"`
	Text   string    `json:"text"`
	Vector []float64 `json:"vector"`
}

type foundChunk struct {
	Number int
	Title  string
	Text   string
}

type modelGrade struct {
	Number int    `json:"number"`
	Grade  string `json:"grade"`
}

type modelAnswer struct {
	Answer string       `json:"answer"`
	Grades []modelGrade `json:"grades"`
}

func (client modelClient) extractPDF(ctx context.Context, data []byte) (string, error) {
	var out struct {
		Text string `json:"text"`
	}
	if err := client.postBytes(ctx, "/extract-pdf", "application/pdf", data, &out); err != nil {
		return "", err
	}
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return "", fmt.Errorf("в pdf нет текста")
	}
	return text, nil
}

func (client modelClient) prepare(ctx context.Context, text string) ([]preparedChunk, error) {
	var out struct {
		Chunks []preparedChunk `json:"chunks"`
	}
	if err := client.post(ctx, "/prepare", map[string]string{"text": text}, &out); err != nil {
		return nil, err
	}
	for _, chunk := range out.Chunks {
		if chunk.Number < 1 || strings.TrimSpace(chunk.Text) == "" || len(chunk.Vector) != embeddingSize {
			return nil, fmt.Errorf("модель вернула некорректный фрагмент")
		}
	}
	return out.Chunks, nil
}

func (client modelClient) embedQuery(ctx context.Context, question string) ([]float64, error) {
	var out struct {
		Vector []float64 `json:"vector"`
	}
	if err := client.post(ctx, "/embed-query", map[string]string{"text": question}, &out); err != nil {
		return nil, err
	}
	if len(out.Vector) != embeddingSize {
		return nil, fmt.Errorf("модель вернула вектор другой длины")
	}
	return out.Vector, nil
}

func (client modelClient) answer(ctx context.Context, question string, chunks []foundChunk) (modelAnswer, error) {
	payloadChunks := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		payloadChunks = append(payloadChunks, map[string]any{
			"number": chunk.Number,
			"text":   chunk.Text,
		})
	}
	var out modelAnswer
	err := client.post(ctx, "/answer", map[string]any{
		"question": question,
		"chunks":   payloadChunks,
	}, &out)
	return out, err
}

func (client modelClient) post(ctx context.Context, path string, payload any, dest any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return client.postBytes(ctx, path, "application/json", raw, dest)
}

func (client modelClient) postBytes(ctx context.Context, path string, contentType string, raw []byte, dest any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", contentType)
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("программа моделей недоступна")
	}
	defer func() { _ = response.Body.Close() }()
	body, err := readLimited(response.Body, 128<<20)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		if failure.Error == "" {
			failure.Error = "программа моделей не ответила"
		}
		return fmt.Errorf("%s", failure.Error)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("программа моделей вернула не JSON")
	}
	return nil
}

func readLimited(source io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(source, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("документ слишком большой для нарезки")
	}
	return body, nil
}
