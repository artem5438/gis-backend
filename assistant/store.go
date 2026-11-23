package assistant

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gis-backend/config"
	"gis-backend/db"
	"gis-backend/models"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var errNotFound = errors.New("документ не найден")

type storedDocument struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	Author    *string   `json:"author"`
	CanEdit   bool      `json:"can_edit"`
}

type documentRecord struct {
	ID         uint
	Title      string
	ObjectKey  string
	CreatedAt  time.Time
	UploadedBy *uint
	FullName   *string
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

func saveDocument(ctx context.Context, title, objectKey string, ownerID uint, chunks []preparedChunk) (storedDocument, error) {
	tx := db.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return storedDocument{}, tx.Error
	}
	var document storedDocument
	err := tx.Raw(`
		INSERT INTO assistant_documents (title, object_key, uploaded_by)
		VALUES (?, ?, ?)
		RETURNING id, title, created_at
	`, title, objectKey, ownerID).Scan(&document).Error
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

func listDocuments(ctx context.Context, user models.User) ([]storedDocument, error) {
	var rows []documentRecord
	err := db.DB.WithContext(ctx).Raw(`
		SELECT d.id, d.title, d.created_at, d.uploaded_by, u.full_name
		FROM assistant_documents AS d
		LEFT JOIN users AS u ON u.id = d.uploaded_by
		ORDER BY d.id DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	documents := make([]storedDocument, 0, len(rows))
	for _, row := range rows {
		author := row.FullName
		if author != nil && strings.TrimSpace(*author) == "" {
			author = nil
		}
		documents = append(documents, storedDocument{
			ID:        row.ID,
			Title:     row.Title,
			CreatedAt: row.CreatedAt,
			Author:    author,
			CanEdit:   canEdit(user, row.UploadedBy),
		})
	}
	return documents, nil
}

func documentByID(ctx context.Context, id uint) (documentRecord, error) {
	var row documentRecord
	err := db.DB.WithContext(ctx).Raw(`
		SELECT id, title, object_key, uploaded_by
		FROM assistant_documents
		WHERE id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return documentRecord{}, err
	}
	if row.ID == 0 {
		return documentRecord{}, errNotFound
	}
	return row, nil
}

func renameDocument(ctx context.Context, id uint, title string) error {
	result := db.DB.WithContext(ctx).Exec(`
		UPDATE assistant_documents
		SET title = ?
		WHERE id = ?
	`, title, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errNotFound
	}
	return nil
}

func removeDocument(ctx context.Context, id uint) error {
	result := db.DB.WithContext(ctx).Exec(`DELETE FROM assistant_documents WHERE id = ?`, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errNotFound
	}
	return nil
}

func putObject(ctx context.Context, key string, data []byte, contentType string) error {
	client, err := minioClient()
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
	_ = deleteObject(ctx, key)
}

func deleteObject(ctx context.Context, key string) error {
	client, err := minioClient()
	if err != nil {
		return fmt.Errorf("файл не удалился из хранилища")
	}
	err = client.RemoveObject(ctx, config.MinioBucket(), key, minio.RemoveObjectOptions{})
	if err == nil {
		return nil
	}
	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.StatusCode == 404 {
		return nil
	}
	return fmt.Errorf("файл не удалился из хранилища")
}

func minioClient() (*minio.Client, error) {
	return minio.New(config.MinioEndpoint(), &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinioAccessKey(), config.MinioSecretKey(), ""),
		Secure: config.MinioUseSSL(),
	})
}
