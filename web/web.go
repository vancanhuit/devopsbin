// Package web exposes the static assets that make up the DevOpsBin console
// SPA, embedded into the binary at compile time.
//
// The Vite + Svelte + Tailwind v4 toolchain in this directory produces a
// `dist/` directory containing:
//
//   - index.html               the SPA shell, references hashed assets
//   - assets/index-<hash>.js   Vite-bundled application code
//   - assets/index-<hash>.css  Tailwind-processed styles
//
// Re-run `bun run build` (in web/) after touching anything under `web/src/`
// or `web/index.html`. The Go compile fails with "no matching files" if
// `web/dist/` is missing -- it is a //go:embed target with `all:`.
package web

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the SPA build output rooted at `dist/`.
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Unreachable: the embed directive guarantees the directory exists
		// at compile time.
		panic(fmt.Errorf("web: locate dist dir: %w", err))
	}
	return sub
}

// IndexHTML returns the bytes of `dist/index.html`, used as the SPA shell.
func IndexHTML() ([]byte, error) {
	b, err := fs.ReadFile(distFS, "dist/index.html")
	if err != nil {
		return nil, fmt.Errorf("web: read dist/index.html: %w", err)
	}
	return b, nil
}

// SwaggerUIFS returns the embedded Swagger UI console rooted at `dist/swagger/`
// (its index.html plus the vendored swagger-ui bundle and stylesheet). The
// directory is produced by the web build's vendor step (scripts/vendor-docs.mjs).
func SwaggerUIFS() (fs.FS, error) {
	sub, err := fs.Sub(distFS, "dist/swagger")
	if err != nil {
		return nil, fmt.Errorf("web: locate dist/swagger dir: %w", err)
	}
	return sub, nil
}

// RedocFS returns the embedded Redoc console rooted at `dist/redoc/` (its
// index.html plus the vendored redoc standalone bundle). The directory is
// produced by the web build's vendor step (scripts/vendor-docs.mjs).
func RedocFS() (fs.FS, error) {
	sub, err := fs.Sub(distFS, "dist/redoc")
	if err != nil {
		return nil, fmt.Errorf("web: locate dist/redoc dir: %w", err)
	}
	return sub, nil
}
