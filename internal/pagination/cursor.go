package pagination

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	cursorVersion    = 1
	DefaultLifetime  = 24 * time.Hour
	maximumKeyIDSize = 32
	maximumRouteSize = 64
)

var (
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrInvalidCodec  = errors.New("invalid cursor codec configuration")
)

type Direction string

const (
	DirectionNext     Direction = "next"
	DirectionPrevious Direction = "previous"
)

type Key struct {
	ID     string
	Secret []byte
}

type CodecConfig struct {
	Active   Key
	Previous *Key
	Clock    func() time.Time
	Lifetime time.Duration
}

type Codec struct {
	active   Key
	keys     map[string][]byte
	clock    func() time.Time
	lifetime time.Duration
}

type Binding struct {
	RouteID        string
	OrganizationID *uuid.UUID
	TeamID         *uuid.UUID
}

type SortKey struct {
	Timestamp time.Time
	ID        uuid.UUID
}

type Cursor struct {
	Direction Direction
	Sort      SortKey
}

type claims struct {
	Version        int       `json:"v"`
	RouteID        string    `json:"r"`
	OrganizationID string    `json:"o,omitempty"`
	TeamID         string    `json:"t,omitempty"`
	Direction      Direction `json:"d"`
	Timestamp      int64     `json:"s"`
	TieBreaker     string    `json:"i"`
	ExpiresAt      int64     `json:"e"`
}

func NewCodec(config CodecConfig) (*Codec, error) {
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.Lifetime == 0 {
		config.Lifetime = DefaultLifetime
	}
	if !validKey(config.Active) || config.Lifetime <= 0 || config.Lifetime > DefaultLifetime {
		return nil, ErrInvalidCodec
	}
	codec := &Codec{
		active: Key{ID: config.Active.ID, Secret: append([]byte(nil), config.Active.Secret...)},
		keys:   map[string][]byte{config.Active.ID: append([]byte(nil), config.Active.Secret...)},
		clock:  config.Clock, lifetime: config.Lifetime,
	}
	if config.Previous != nil {
		if !validKey(*config.Previous) || config.Previous.ID == config.Active.ID {
			return nil, ErrInvalidCodec
		}
		codec.keys[config.Previous.ID] = append([]byte(nil), config.Previous.Secret...)
	}
	return codec, nil
}

func (codec *Codec) Issue(binding Binding, cursor Cursor) (string, error) {
	if !validBinding(binding) || !validDirection(cursor.Direction) ||
		cursor.Sort.Timestamp.IsZero() || cursor.Sort.ID == uuid.Nil {
		return "", ErrInvalidCursor
	}
	now := codec.clock().UTC()
	payload := claims{
		Version: cursorVersion, RouteID: binding.RouteID,
		Direction: cursor.Direction, Timestamp: cursor.Sort.Timestamp.UTC().UnixMicro(),
		TieBreaker: cursor.Sort.ID.String(), ExpiresAt: now.Add(codec.lifetime).Unix(),
	}
	if binding.OrganizationID != nil {
		payload.OrganizationID = binding.OrganizationID.String()
	}
	if binding.TeamID != nil {
		payload.TeamID = binding.TeamID.String()
	}
	encodedClaims, err := json.Marshal(payload)
	if err != nil {
		return "", ErrInvalidCursor
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(encodedClaims)
	signed := codec.active.ID + "." + payloadPart
	signature := sign(codec.active.Secret, signed)
	token := signed + "." + base64.RawURLEncoding.EncodeToString(signature)
	if len(token) > MaximumTokenSize {
		return "", ErrInvalidCursor
	}
	return token, nil
}

func (codec *Codec) Verify(token string, binding Binding) (Cursor, error) {
	if len(token) == 0 || len(token) > MaximumTokenSize || !validBinding(binding) {
		return Cursor{}, ErrInvalidCursor
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || !validKeyID(parts[0]) || parts[1] == "" || parts[2] == "" {
		return Cursor{}, ErrInvalidCursor
	}
	secret, exists := codec.keys[parts[0]]
	if !exists {
		return Cursor{}, ErrInvalidCursor
	}
	signature, err := decodeCanonical(parts[2])
	if err != nil || len(signature) != sha256.Size ||
		!hmac.Equal(signature, sign(secret, parts[0]+"."+parts[1])) {
		return Cursor{}, ErrInvalidCursor
	}
	encodedClaims, err := decodeCanonical(parts[1])
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	var payload claims
	decoder := json.NewDecoder(strings.NewReader(string(encodedClaims)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	canonicalClaims, err := json.Marshal(payload)
	if err != nil || !hmac.Equal(canonicalClaims, encodedClaims) {
		return Cursor{}, ErrInvalidCursor
	}
	if payload.Version != cursorVersion || payload.RouteID != binding.RouteID ||
		payload.OrganizationID != optionalUUID(binding.OrganizationID) ||
		payload.TeamID != optionalUUID(binding.TeamID) || !validDirection(payload.Direction) {
		return Cursor{}, ErrInvalidCursor
	}
	now := codec.clock().UTC()
	expiresAt := time.Unix(payload.ExpiresAt, 0).UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(codec.lifetime)) {
		return Cursor{}, ErrInvalidCursor
	}
	tieBreaker, err := uuid.Parse(payload.TieBreaker)
	if err != nil || tieBreaker == uuid.Nil || payload.TieBreaker != tieBreaker.String() || payload.Timestamp == 0 {
		return Cursor{}, ErrInvalidCursor
	}
	return Cursor{
		Direction: payload.Direction,
		Sort:      SortKey{Timestamp: time.UnixMicro(payload.Timestamp).UTC(), ID: tieBreaker},
	}, nil
}

func sign(secret []byte, input string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(input))
	return mac.Sum(nil)
}

func decodeCanonical(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, ErrInvalidCursor
	}
	return decoded, nil
}

func validKey(key Key) bool {
	return validKeyID(key.ID) && len(key.Secret) >= 32
}

func validKeyID(keyID string) bool {
	if len(keyID) == 0 || len(keyID) > maximumKeyIDSize {
		return false
	}
	for _, character := range keyID {
		allowed := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '_' || character == '-'
		if !allowed {
			return false
		}
	}
	return true
}

func validBinding(binding Binding) bool {
	if len(binding.RouteID) == 0 || len(binding.RouteID) > maximumRouteSize {
		return false
	}
	if binding.OrganizationID != nil && *binding.OrganizationID == uuid.Nil {
		return false
	}
	return binding.TeamID == nil || *binding.TeamID != uuid.Nil
}

func validDirection(direction Direction) bool {
	return direction == DirectionNext || direction == DirectionPrevious
}

func optionalUUID(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}
