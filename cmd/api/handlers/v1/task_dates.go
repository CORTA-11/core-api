package v1

import (
	"bytes"
	"encoding/json"
	"time"
)

func decodeTaskDate(raw json.RawMessage) (*time.Time, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true, nil
	}
	var value time.Time
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, true, err
	}
	return &value, true, nil
}
