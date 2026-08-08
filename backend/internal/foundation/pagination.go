package foundation

type PageRequest struct {
	Cursor string
	Limit  int
}

type PageResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	PageCount  int    `json:"pageCount"`
}
