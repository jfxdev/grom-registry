package foundation

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type PageRequest struct {
	Cursor string
	Limit  int
	Scope  string
}

type PageResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	PageCount  int    `json:"pageCount"`
}

// PageCursor is an opaque, scoped continuation token. Offset remains for
// legacy in-memory lists; persistent lists use Timestamp and ID as a keyset.
type PageCursor struct {
	Version   int    `json:"v"`
	Scope     string `json:"s"`
	Offset    int    `json:"o,omitempty"`
	Timestamp string `json:"t,omitempty"`
	ID        string `json:"i,omitempty"`
	Marker    string `json:"m,omitempty"`
}

func EncodePageCursor(cursor PageCursor) (string, error) {
	cursor.Version = 1
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodePageCursor(raw, scope string) (PageCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return PageCursor{}, fmt.Errorf("invalid cursor")
	}
	var cursor PageCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 || (scope != "" && cursor.Scope != scope) || cursor.Offset < 0 {
		return PageCursor{}, fmt.Errorf("invalid cursor")
	}
	return cursor, nil
}
