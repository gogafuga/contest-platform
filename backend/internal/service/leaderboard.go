package service

import (
	"context"
	"encoding/json"

	dbsqlc "devsec-contest01/internal/db/sqlc"
)

type LeaderboardEntry struct {
	Name   string
	Group  string
	Scores map[string]float64
	Total  float64
}

type Leaderboard struct {
	q dbsqlc.Querier
}

func NewLeaderboard(q dbsqlc.Querier) *Leaderboard {
	return &Leaderboard{q: q}
}

// List returns every participant with their per-task best scores and total,
// already sorted desc by total (then name asc) by the underlying query.
func (l *Leaderboard) List(ctx context.Context) ([]LeaderboardEntry, error) {
	rows, err := l.q.ListLeaderboard(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]LeaderboardEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, LeaderboardEntry{
			Name:   row.Name,
			Group:  row.GroupName,
			Scores: scoresFromJSONB(row.Scores),
			Total:  row.Total,
		})
	}
	return entries, nil
}

// scoresFromJSONB normalizes the jsonb_object_agg result regardless of which
// concrete Go type pgx decoded it into.
func scoresFromJSONB(raw interface{}) map[string]float64 {
	out := make(map[string]float64)
	switch v := raw.(type) {
	case map[string]interface{}:
		for k, val := range v {
			if f, ok := val.(float64); ok {
				out[k] = f
			}
		}
	case []byte:
		var m map[string]float64
		if json.Unmarshal(v, &m) == nil {
			out = m
		}
	case string:
		var m map[string]float64
		if json.Unmarshal([]byte(v), &m) == nil {
			out = m
		}
	}
	return out
}
