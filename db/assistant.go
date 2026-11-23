package db

// ensureAssistant creates the document tables used by the assistant.
// Vectors stay in this PostgreSQL through pgvector. The column width is the e5-small vector, 384.
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
			embedding vector(384) NOT NULL
		)
	`).Error
	if err != nil {
		return err
	}
	return DB.Exec(`
		CREATE INDEX IF NOT EXISTS assistant_chunks_embedding_hnsw
		ON assistant_chunks USING hnsw (embedding vector_cosine_ops)
	`).Error
}
