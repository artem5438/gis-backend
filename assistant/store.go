package assistant

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gis-backend/config"
	"gis-backend/db"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type storedDocument struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

func vectorLiteral(values []float64) string {
	var builder strings.Builder
	builder.WriteByte('[')
	for index, value := range values {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	}
	builder.WriteByte(']')
	return builder.String()
}

func saveDocument(ctx context.Context, title, objectKey string, chunks []preparedChunk) (storedDocument, error) {
	tx := db.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return storedDocument{}, tx.Error
	}
	var document storedDocument
	err := tx.Raw(`
		INSERT INTO assistant_documents (title, object_key)
		VALUES (?, ?)
		RETURNING id, title, created_at
	`, title, objectKey).Scan(&document).Error
	if err != nil {
		tx.Rollback()
		return storedDocument{}, err
	}
	for _, chunk := range chunks {
		err = tx.Exec(`
			INSERT INTO assistant_chunks (document_id, number, body, embedding)
			VALUES (?, ?, ?, CAST(? AS vector))
		`, document.ID, chunk.Number, chunk.Text, vectorLiteral(chunk.Vector)).Error
		if err != nil {
			tx.Rollback()
			return storedDocument{}, err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return storedDocument{}, err
	}
	return document, nil
}

func nearestChunks(ctx context.Context, vector []float64) ([]foundChunk, error) {
	var rows []struct {
		Number int
		Title  string
		Body   string
	}
	err := db.DB.WithContext(ctx).Raw(`
		SELECT c.number, d.title, c.body
		FROM assistant_chunks AS c
		JOIN assistant_documents AS d ON d.id = c.document_id
		ORDER BY c.embedding <=> CAST(? AS vector)
		LIMIT 3
	`, vectorLiteral(vector)).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	found := make([]foundChunk, 0, len(rows))
	for _, row := range rows {
		found = append(found, foundChunk{Number: row.Number, Title: row.Title, Text: row.Body})
	}
	return found, nil
}

func listDocuments(ctx context.Context) ([]storedDocument, error) {
	var documents []storedDocument
	err := db.DB.WithContext(ctx).Raw(`
		SELECT id, title, created_at
		FROM assistant_documents
		ORDER BY id DESC
	`).Scan(&documents).Error
	return documents, err
}

func putObject(ctx context.Context, key string, data []byte, contentType string) error {
	client, err := minio.New(config.MinioEndpoint(), &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinioAccessKey(), config.MinioSecretKey(), ""),
		Secure: config.MinioUseSSL(),
	})
	if err != nil {
		return err
	}
	_, err = client.PutObject(
		ctx,
		config.MinioBucket(),
		key,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return fmt.Errorf("файл не записался в хранилище")
	}
	return nil
}

func removeObject(ctx context.Context, key string) {
	client, err := minio.New(config.MinioEndpoint(), &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinioAccessKey(), config.MinioSecretKey(), ""),
		Secure: config.MinioUseSSL(),
	})
	if err != nil {
		return
	}
	_ = client.RemoveObject(ctx, config.MinioBucket(), key, minio.RemoveObjectOptions{})
}
