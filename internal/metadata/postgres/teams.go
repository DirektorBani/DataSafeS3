package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Store) PutTeam(rec metadata.TeamRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO teams (id, name, created_at) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, rec.ID, rec.Name, rec.CreatedAt)
	return err
}

func (s *Store) GetTeam(id string) (metadata.TeamRecord, error) {
	var rec metadata.TeamRecord
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, name, created_at FROM teams WHERE id=$1`, id).Scan(&rec.ID, &rec.Name, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) ListTeams() ([]metadata.TeamRecord, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT id, name, created_at FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.TeamRecord
	for rows.Next() {
		var rec metadata.TeamRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTeam(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM teams WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) AddUserTeam(userID, teamID string) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO user_teams (user_id, team_id) VALUES ($1,$2)
		ON CONFLICT DO NOTHING`, userID, teamID)
	return err
}

func (s *Store) RemoveUserTeam(userID, teamID string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM user_teams WHERE user_id=$1 AND team_id=$2`, userID, teamID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ListUserTeamIDs(userID string) ([]string, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT team_id FROM user_teams WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) ListBucketsFiltered(filter metadata.BucketListFilter) ([]metadata.BucketRecord, error) {
	all, err := s.ListBuckets()
	if err != nil {
		return nil, err
	}
	if filter.Unfiltered {
		return all, nil
	}
	var out []metadata.BucketRecord
	for _, b := range all {
		if metadata.BucketMatchesFilter(b, filter) {
			out = append(out, b)
		}
	}
	return out, nil
}
