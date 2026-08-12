//go:build embedruntime

package engine

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Bundled lemonade-sdk Windows gfx1151 ROCm zip (downloaded at release build time).
//
//go:embed embedded/runtime.zip
var embeddedROCmFS embed.FS

//go:embed embedded/runtime.tag
var embeddedROCmTagRaw string

// HasEmbeddedROCm is true for all-in-one release builds.
func HasEmbeddedROCm() bool { return true }

func extractEmbeddedROCm(destDir string) (tag string, err error) {
	tag = strings.TrimSpace(embeddedROCmTagRaw)
	if tag == "" {
		tag = "embedded"
	}
	zipPath := filepath.Join(destDir, "runtime-embedded.zip")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	src, err := embeddedROCmFS.Open("embedded/runtime.zip")
	if err != nil {
		return "", fmt.Errorf("open embedded runtime: %w", err)
	}
	defer src.Close()

	out, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(zipPath)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(zipPath)
		return "", closeErr
	}
	fmt.Printf("Extracting bundled ROCm runtime (%s, %d MB)...\n", tag, n/(1024*1024))
	extractDir := filepath.Join(destDir, tag+"-rocm-gfx1151")
	if err := os.RemoveAll(extractDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	if err := unzip(zipPath, extractDir); err != nil {
		_ = os.Remove(zipPath)
		return "", err
	}
	_ = os.Remove(zipPath)
	return tag, nil
}
