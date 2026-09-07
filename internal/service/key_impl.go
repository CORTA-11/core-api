package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	minimumPublicKeyLength = 64
	maximumPublicKeyLength = 8192

	minimumEncryptedPrivateKeyLength = 64
	maximumEncryptedPrivateKeyLength = 16384
	minimumKEKSaltLength             = 8
	maximumKEKSaltLength             = 256
	minimumKEKIterations             = 10000
	maximumKEKIterations             = 10000000

	defaultTeamKeyAlgorithm = "aes-256-gcm"
	rsaWrapAlgorithm        = "rsa-oaep-2048"
	pbkdf2Algorithm         = "pbkdf2-sha256"
)

type keyService struct {
	pool       pgxPool
	authorizer applicationAuthorizer
}

// NewKeyService creates a new instance of KeyService.
func NewKeyService(pool pgxPool, authorizer applicationAuthorizer) KeyService {
	return &keyService{
		pool:       pool,
		authorizer: authorizer,
	}
}

// UpsertUserKeys upserts the caller's key pair: public key always, password-sealed
// private key only when supplied (the SQL coalesces over the existing value).
func (s *keyService) UpsertUserKeys(ctx context.Context, p session.Principal, input UserKeyUpdate) (*UserKey, error) {
	if p.UserID == uuid.Nil {
		return nil, authorization.ErrUnauthenticated
	}
	if len(input.PublicKey) < minimumPublicKeyLength || len(input.PublicKey) > maximumPublicKeyLength {
		return nil, ErrInvalidInput
	}
	if err := validatePrivateKeyMaterial(input); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	row, err := publicdb.New(tx).UpsertUserKeys(ctx, publicdb.UpsertUserKeysParams{
		UserID:              p.UserID,
		PublicKey:           input.PublicKey,
		EncryptedPrivateKey: pgtextParam(input.EncryptedPrivateKey),
		KekSalt:             pgtextParam(input.KEKSalt),
		KekIterations:       pgint4Param(input.KEKIterations),
		KekAlgorithm:        optionalText(input.KEKAlgorithm, pbkdf2Algorithm),
	})
	if err != nil {
		return nil, err
	}

	return userKeyFrom(&publicdb.UserPublicKey{
		UserID:              row.UserID,
		PublicKey:           row.PublicKey,
		EncryptedPrivateKey: row.EncryptedPrivateKey,
		KekSalt:             row.KekSalt,
		KekIterations:       row.KekIterations,
		KekAlgorithm:        row.KekAlgorithm,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}), tx.Commit(ctx)
}

// GetUserKeys gets the caller's own key material.
func (s *keyService) GetUserKeys(ctx context.Context, p session.Principal) (*UserKey, error) {
	if p.UserID == uuid.Nil {
		return nil, authorization.ErrUnauthenticated
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	row, err := publicdb.New(tx).GetUserPublicKey(ctx, p.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, authorization.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}

	return userKeyFrom(&publicdb.UserPublicKey{
		UserID:              row.UserID,
		PublicKey:           row.PublicKey,
		EncryptedPrivateKey: row.EncryptedPrivateKey,
		KekSalt:             row.KekSalt,
		KekIterations:       row.KekIterations,
		KekAlgorithm:        row.KekAlgorithm,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}), tx.Commit(ctx)
}

// GetPublicKeysForTeam gets public keys for team.
func (s *keyService) GetPublicKeysForTeam(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]UserPublicKey, error) {
	var memberUUIDs []uuid.UUID
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionTeamRead, func(queries *tenantdb.Queries) error {
		rows, err := queries.ListTeamMembersAfter(ctx, 1000)
		if err != nil {
			return err
		}
		memberUUIDs = make([]uuid.UUID, len(rows))
		for i, row := range rows {
			memberUUIDs[i] = row.UserPublicID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	rows, err := publicdb.New(tx).GetUserPublicKeys(ctx, publicdb.GetUserPublicKeysParams{
		UserIds: memberUUIDs,
		Limit:   maximumListResults,
	})
	if err != nil {
		return nil, err
	}

	views := make([]UserPublicKey, len(rows))
	for i, row := range rows {
		views[i] = UserPublicKey{
			UserID:    row.UserID,
			PublicKey: row.PublicKey,
			CreatedAt: row.CreatedAt,
		}
	}

	return views, tx.Commit(ctx)
}

// CreateTeamKey creates the next team key version, superseding the active one.
func (s *keyService) CreateTeamKey(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, input TeamKeyVersionInput) (*TeamKey, error) {
	if len(input.Wraps) == 0 {
		return nil, ErrInvalidInput
	}
	if input.Algorithm == "" {
		input.Algorithm = defaultTeamKeyAlgorithm
	}
	if input.Algorithm != defaultTeamKeyAlgorithm {
		return nil, ErrInvalidInput
	}
	if !validWraps(input.Wraps) {
		return nil, ErrInvalidInput
	}
	wrapped, ok := includesUser(input.Wraps, p.UserID)
	if !ok {
		return nil, ErrInvalidInput
	}

	wrapsJSON, err := json.Marshal(wrapped)
	if err != nil {
		return nil, err
	}

	var created tenantdb.TeamKey
	err = s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileUpload, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		created, err = queries.CreateTeamKeyVersion(ctx, tenantdb.CreateTeamKeyVersionParams{
			CandidateTeamID: resolvedTeam.ID,
			KeyAlgorithm:    input.Algorithm,
			KeyWraps:        wrapsJSON,
			KeyCreatedBy:    p.UserID,
		})
		return err
	})
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}

	view := s.teamKeyFrom(&created, teamID, p.UserID)
	return &view, nil
}

// ListTeamKeys lists team key versions visible to the caller (RLS limits rows to
// those wrapped for them); each row's wraps are filtered to the caller.
func (s *keyService) ListTeamKeys(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]TeamKey, error) {
	var rows []tenantdb.TeamKey
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionTeamRead, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		rows, err = queries.ListTeamKeysForTeam(ctx, tenantdb.ListTeamKeysForTeamParams{
			TeamID: resolvedTeam.ID,
			Limit:  maximumListResults,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	views := make([]TeamKey, len(rows))
	for i := range rows {
		views[i] = s.teamKeyFrom(&rows[i], teamID, p.UserID)
	}
	return views, nil
}

func (s *keyService) teamKeyFrom(row *tenantdb.TeamKey, teamID uuid.UUID, caller uuid.UUID) TeamKey {
	var all []TeamKeyWrap
	if len(row.Wraps) > 0 {
		_ = json.Unmarshal(row.Wraps, &all)
	}

	own := make([]TeamKeyWrap, 0, len(all))
	wrappedUsers := make([]uuid.UUID, 0, len(all))
	for _, wrap := range all {
		if wrap.UserID == caller {
			own = append(own, wrap)
		}
		wrappedUsers = append(wrappedUsers, wrap.UserID)
	}

	return TeamKey{
		ID:             row.ID,
		TeamID:         teamID,
		Version:        row.Version,
		Status:         row.Status,
		Algorithm:      row.Algorithm,
		Wraps:          own,
		WrappedUserIDs: wrappedUsers,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
	}
}

func validatePrivateKeyMaterial(input UserKeyUpdate) error {
	hasAny := input.EncryptedPrivateKey != nil || input.KEKSalt != nil || input.KEKIterations != nil || input.KEKAlgorithm != nil
	if !hasAny {
		return nil
	}
	if input.EncryptedPrivateKey == nil || input.KEKSalt == nil || input.KEKIterations == nil || input.KEKAlgorithm == nil {
		return ErrInvalidInput
	}
	if len(*input.EncryptedPrivateKey) < minimumEncryptedPrivateKeyLength || len(*input.EncryptedPrivateKey) > maximumEncryptedPrivateKeyLength {
		return ErrInvalidInput
	}
	if len(*input.KEKSalt) < minimumKEKSaltLength || len(*input.KEKSalt) > maximumKEKSaltLength {
		return ErrInvalidInput
	}
	if *input.KEKIterations < minimumKEKIterations || *input.KEKIterations > maximumKEKIterations {
		return ErrInvalidInput
	}
	if *input.KEKAlgorithm != pbkdf2Algorithm {
		return ErrInvalidInput
	}
	return nil
}

func validWraps(wraps []TeamKeyWrap) bool {
	for _, wrap := range wraps {
		if wrap.UserID == uuid.Nil {
			return false
		}
		if len(wrap.Key) < minimumPublicKeyLength || len(wrap.Key) > maximumPublicKeyLength {
			return false
		}
		if wrap.Algorithm != rsaWrapAlgorithm {
			return false
		}
	}
	return true
}

func includesUser(wraps []TeamKeyWrap, userID uuid.UUID) ([]TeamKeyWrap, bool) {
	for _, wrap := range wraps {
		if wrap.UserID == userID {
			return wraps, true
		}
	}
	return wraps, false
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func pgtextParam(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func pgint4Param(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func optionalText(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

func userKeyFrom(row *publicdb.UserPublicKey) *UserKey {
	key := &UserKey{
		UserID:              row.UserID,
		PublicKey:           row.PublicKey,
		EncryptedPrivateKey: nil,
		KEKSalt:             nil,
		KEKIterations:       nil,
		KEKAlgorithm:        &row.KekAlgorithm,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
	if row.EncryptedPrivateKey.Valid {
		key.EncryptedPrivateKey = &row.EncryptedPrivateKey.String
	}
	if row.KekSalt.Valid {
		key.KEKSalt = &row.KekSalt.String
	}
	if row.KekIterations.Valid {
		iterations := row.KekIterations.Int32
		key.KEKIterations = &iterations
	}
	return key
}
