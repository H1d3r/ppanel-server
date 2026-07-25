package repository

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// NormalizePage clamps pagination inputs; module repo implementations share it.
func NormalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return page, size
}

// NormalizePageFloor clamps pagination inputs without a minimum page size.
func NormalizePageFloor(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPageSize
	}
	return page, size
}
