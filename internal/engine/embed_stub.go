//go:build !embedruntime

package engine

import "errors"

// HasEmbeddedROCm is false for normal (non-release) builds.
func HasEmbeddedROCm() bool { return false }

func extractEmbeddedROCm(_ string) (tag string, err error) {
	return "", errors.New("no embedded ROCm runtime in this build")
}
