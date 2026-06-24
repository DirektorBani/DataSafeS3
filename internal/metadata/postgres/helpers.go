package postgres

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func marshalJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

func unmarshalJSON(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tt := t.Time
	return &tt
}

func timestamptzPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil || t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func optionalText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func jsonMap(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var m map[string]string
	_ = json.Unmarshal(data, &m)
	return m
}

func jsonStringSlice(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var s []string
	_ = json.Unmarshal(data, &s)
	return s
}
