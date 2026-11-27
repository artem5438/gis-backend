package db

func ensureReports() error {
	return DB.Exec(`
		CREATE TABLE IF NOT EXISTS parcel_reports (
			id bigserial PRIMARY KEY,
			user_id bigint NOT NULL REFERENCES users(id),
			format text NOT NULL,
			cadastral_number text NOT NULL,
			include_score boolean NOT NULL,
			include_quarter boolean NOT NULL,
			include_charts boolean NOT NULL,
			body jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)
	`).Error
}
