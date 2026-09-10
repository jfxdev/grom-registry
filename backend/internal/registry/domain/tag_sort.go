package domain

// TagSort selects the ordering applied when listing or searching tags.
type TagSort string

const (
	TagSortNewest   TagSort = "newest"
	TagSortOldest   TagSort = "oldest"
	TagSortNameAsc  TagSort = "name-asc"
	TagSortNameDesc TagSort = "name-desc"
)
