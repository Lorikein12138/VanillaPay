package migrations

import (
	"embed"
	"io/fs"
)

//go:embed all:*
var files embed.FS

// FS returns the versioned Goose migration files embedded in the server.
func FS() fs.FS {
	return files
}
