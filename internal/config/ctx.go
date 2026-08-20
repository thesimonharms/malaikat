package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// NativeCtxQwen35 is the trained window for Qwen3.5/3.6 35B-A3B and Ornith-1.5-35B-A3B.
	NativeCtxQwen35 = 262144

	Ctx256K = 256 * 1024  // 262144
	Ctx512K = 512 * 1024  // 524288
	Ctx1M   = 1024 * 1024 // 1048576
)

// CtxSize is a context length. YAML/JSON may be an integer or a size string
// such as "256k", "512k", "1m". 0 means "use the model's trained maximum".
type CtxSize int

func (c CtxSize) Int() int { return int(c) }

func (c *CtxSize) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("ctx_size: expected a scalar")
	}
	n, err := ParseCtxSize(value.Value)
	if err != nil {
		return err
	}
	*c = CtxSize(n)
	return nil
}

func (c CtxSize) MarshalYAML() (interface{}, error) {
	return int(c), nil
}

func (c *CtxSize) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*c = CtxSize(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("ctx_size: %w", err)
	}
	n, err := ParseCtxSize(s)
	if err != nil {
		return err
	}
	*c = CtxSize(n)
	return nil
}

func (c CtxSize) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(c))
}

// ParseCtxSize accepts 256k / 512k / 1m aliases, raw token counts, and 0/max.
func ParseCtxSize(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSuffix(s, "tokens")
	s = strings.TrimSuffix(s, "token")
	s = strings.TrimSpace(s)
	switch s {
	case "", "0", "max", "native", "model":
		return 0, nil
	case "256k", "256kb", "256ki":
		return Ctx256K, nil
	case "512k", "512kb", "512ki":
		return Ctx512K, nil
	case "1m", "1mb", "1mi", "1024k":
		return Ctx1M, nil
	}

	mult := 1
	switch {
	case strings.HasSuffix(s, "ki") || strings.HasSuffix(s, "kb"):
		mult = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "i"), "b")
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "k"):
		mult = 1024
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "mi") || strings.HasSuffix(s, "mb"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "i"), "b")
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "m"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("ctx size %q: use 256k, 512k, 1m, or a token count", s)
	}
	return n * mult, nil
}

// FormatCtxSize is a short label for logs.
func FormatCtxSize(n int) string {
	switch n {
	case 0:
		return "0 (model max)"
	case Ctx256K:
		return "262144 (256k, native)"
	case Ctx512K:
		return "524288 (512k)"
	case Ctx1M:
		return "1048576 (1m)"
	default:
		return strconv.Itoa(n)
	}
}
