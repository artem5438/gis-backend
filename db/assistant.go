package db

// ensureAssistant creates the document tables used by the assistant.
// Vectors stay in this PostgreSQL through pgvector. The column width is the e5-base vector, 768.
func ensureAssistant() error {
	if err := DB.Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
		return err
	}
	err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS assistant_documents (
			id bigserial PRIMARY KEY,
			title text NOT NULL,
			object_key text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)
	`).Error
	if err != nil {
		return err
	}
	err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS assistant_chunks (
			id bigserial PRIMARY KEY,
			document_id bigint NOT NULL REFERENCES assistant_documents(id) ON DELETE CASCADE,
			number integer NOT NULL,
			body text NOT NULL,
			embedding vector(768) NOT NULL
		)
	`).Error
	if err != nil {
		return err
	}
	if err := widenAssistantVectors(); err != nil {
		return err
	}
	return DB.Exec(`
		CREATE INDEX IF NOT EXISTS assistant_chunks_embedding_hnsw
		ON assistant_chunks USING hnsw (embedding vector_cosine_ops)
	`).Error
}

// widenAssistantVectors moves an existing e5-small column to 768 numbers.
// Old vectors cannot be cast, so the fragment rows are cleared and must be uploaded again.
func widenAssistantVectors() error {
	var dims int
	err := DB.Raw(`
		SELECT atttypmod
		FROM pg_attribute
		WHERE attrelid = 'assistant_chunks'::regclass
			AND attname = 'embedding'
			AND NOT attisdropped
	`).Scan(&dims).Error
	if err != nil {
		return err
	}
	if dims == 768 || dims <= 0 {
		return nil
	}
	if err := DB.Exec(`DROP INDEX IF EXISTS assistant_chunks_embedding_hnsw`).Error; err != nil {
		return err
	}
	if err := DB.Exec(`DELETE FROM assistant_chunks`).Error; err != nil {
		return err
	}
	return DB.Exec(`ALTER TABLE assistant_chunks ALTER COLUMN embedding TYPE vector(768)`).Error
}
