package service

import "strings"

var allowedOrderBy = map[string]string{
	"created_at": "created_at",
	"updated_at": "updated_at",
	"name":       "name",
	"size":       "size",
	"id":         "id",
}

func sanitizeOrderBy(orderBy string) string {
	key := strings.ToLower(strings.TrimSpace(orderBy))
	return allowedOrderBy[key]
}
