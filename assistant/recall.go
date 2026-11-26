package assistant

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"gis-backend/db"
)

func recallChunks(ctx context.Context, vector []float64, question string) ([]foundChunk, error) {
	nearest, err := nearestChunks(ctx, vector)
	if err != nil {
		return nil, err
	}
	literal, err := wordChunks(ctx, queryWords(question))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	merged := make([]foundChunk, 0, len(nearest)+len(literal))
	for _, chunk := range append(nearest, literal...) {
		key := chunk.Title + "\n" + chunk.Text
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, chunk)
	}
	return merged, nil
}

func queryWords(question string) []string {
	text := strings.ToLower(strings.ReplaceAll(question, "ё", "е"))
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	stop := map[string]bool{
		"скажи": true, "мне": true, "какая": true, "какой": true, "какие": true,
		"кто": true, "что": true, "как": true, "это": true, "для": true, "про": true,
		"или": true, "его": true, "ее": true, "чем": true, "могу": true, "есть": true,
	}
	seen := map[string]bool{}
	words := make([]string, 0, 6)
	for _, word := range fields {
		if stop[word] || utf8.RuneCountInString(word) < 4 {
			continue
		}
		runes := []rune(word)
		if len(runes) > 6 {
			word = string(runes[:6])
		}
		if seen[word] {
			continue
		}
		seen[word] = true
		words = append(words, word)
		if len(words) == 6 {
			break
		}
	}
	return words
}

func wordChunks(ctx context.Context, words []string) ([]foundChunk, error) {
	if len(words) == 0 {
		return nil, nil
	}
	var query strings.Builder
	args := make([]any, 0, len(words)*2)
	query.WriteString(`SELECT c.number, d.id AS document_id, d.title, d.object_key, c.body FROM assistant_chunks AS c `)
	query.WriteString(`JOIN assistant_documents AS d ON d.id = c.document_id WHERE (`)
	for index, word := range words {
		if index > 0 {
			query.WriteString(` OR `)
		}
		query.WriteString(`lower(c.body) LIKE ?`)
		args = append(args, "%"+word+"%")
	}
	query.WriteString(`) ORDER BY (`)
	for index, word := range words {
		if index > 0 {
			query.WriteString(` + `)
		}
		query.WriteString(`CASE WHEN lower(c.body) LIKE ? THEN 1 ELSE 0 END`)
		args = append(args, "%"+word+"%")
	}
	query.WriteString(`) DESC, c.id LIMIT 3`)
	var rows []struct {
		Number     int
		DocumentID uint
		Title      string
		ObjectKey  string
		Body       string
	}
	err := db.DB.WithContext(ctx).Raw(query.String(), args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	found := make([]foundChunk, 0, len(rows))
	for _, row := range rows {
		found = append(found, foundChunk{
			Number:     row.Number,
			DocumentID: row.DocumentID,
			Title:      row.Title,
			Kind:       kindOf(row.ObjectKey),
			Text:       row.Body,
		})
	}
	return found, nil
}
