package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "devsec-contest01/internal/db/sqlc"
)

// Participant is the service-layer view of a registered contest participant,
// decoupled from the generated sqlc row types.
type Participant struct {
	ID    int64
	Name  string
	Group string
}

type Auth struct {
	q          dbsqlc.Querier
	sessionTTL time.Duration
}

func NewAuth(q dbsqlc.Querier, sessionTTL time.Duration) *Auth {
	return &Auth{q: q, sessionTTL: sessionTTL}
}

// Register creates a participant and returns a freshly issued opaque session
// token (returned to the caller once, never stored — only its hash is).
func (a *Auth) Register(ctx context.Context, name, group string) (string, Participant, error) {
	name = strings.TrimSpace(name)
	group = strings.TrimSpace(group)
	if len([]rune(name)) < 2 {
		return "", Participant{}, fmt.Errorf("%w: full name must be at least 2 characters", ErrInvalidInput)
	}
	if group == "" {
		return "", Participant{}, fmt.Errorf("%w: group is required", ErrInvalidInput)
	}

	p, err := a.q.CreateParticipant(ctx, dbsqlc.CreateParticipantParams{Name: name, GroupName: group})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", Participant{}, ErrDuplicateName
		}
		return "", Participant{}, err
	}

	token, err := a.issueToken(ctx, p.ID)
	if err != nil {
		return "", Participant{}, err
	}

	return token, Participant{ID: p.ID, Name: p.Name, Group: p.GroupName}, nil
}

// ValidateToken resolves a raw bearer token to its participant, or
// ErrUnauthorized if it's missing, malformed, or expired.
func (a *Auth) ValidateToken(ctx context.Context, token string) (Participant, error) {
	if token == "" {
		return Participant{}, ErrUnauthorized
	}
	row, err := a.q.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		return Participant{}, ErrUnauthorized
	}
	return Participant{ID: row.ParticipantID, Name: row.Name, Group: row.GroupName}, nil
}

// issueTokenRaw generates a new opaque token — 32 random bytes, hex-encoded —
// with no DB interaction, so it's unit-testable without a live Querier.
func (a *Auth) issueTokenRaw() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (a *Auth) issueToken(ctx context.Context, participantID int64) (string, error) {
	token, err := a.issueTokenRaw()
	if err != nil {
		return "", err
	}

	_, err = a.q.CreateSession(ctx, dbsqlc.CreateSessionParams{
		ParticipantID: participantID,
		TokenHash:     hashToken(token),
		ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(a.sessionTTL), Valid: true},
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
