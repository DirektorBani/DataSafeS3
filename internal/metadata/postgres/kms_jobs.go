package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

func (s *Store) PutKMSJob(rec metadata.KMSJob) error {
	doc, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(), `
		INSERT INTO kms_jobs (id, bucket_key, status, doc)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (id) DO UPDATE SET
			bucket_key=EXCLUDED.bucket_key, status=EXCLUDED.status, doc=EXCLUDED.doc`,
		rec.ID, rec.BucketKey, rec.Status, doc)
	return err
}

func (s *Store) GetKMSJob(id string) (metadata.KMSJob, error) {
	var doc []byte
	err := s.pool.QueryRow(context.Background(), `SELECT doc FROM kms_jobs WHERE id=$1`, id).Scan(&doc)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return metadata.KMSJob{}, metadata.ErrNotFound
		}
		return metadata.KMSJob{}, err
	}
	var rec metadata.KMSJob
	if err := json.Unmarshal(doc, &rec); err != nil {
		return metadata.KMSJob{}, err
	}
	return rec, nil
}

func (s *Store) ListKMSJobs() ([]metadata.KMSJob, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT doc FROM kms_jobs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.KMSJob
	for rows.Next() {
		var doc []byte
		if err := rows.Scan(&doc); err != nil {
			return nil, err
		}
		var rec metadata.KMSJob
		if err := json.Unmarshal(doc, &rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
