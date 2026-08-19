package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/simon/malaikat/internal/engine"
	"github.com/simon/malaikat/internal/optimize"
)

func runBench(args []string) error {
	fs := newFlagSet("bench")
	configPath := fs.String("config", "", "JSON/YAML config file")
	model := fs.String("m", "", "GGUF for llama-bench")
	url := fs.String("url", "http://127.0.0.1:8080", "malaikat/llama-server base URL")
	ollamaModel := fs.String("ollama", "", "if set, also time Ollama model name")
	prompt := fs.String("prompt", "Write a Python function that merges two sorted lists. Only code.", "bench prompt")
	maxTokens := fs.Int("n", 128, "max tokens to generate")
	kernel := fs.Bool("kernel", false, "also run llama-bench on -m GGUF")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadMergedConfig(*configPath, *model, nil)
	if err != nil {
		return err
	}

	fmt.Println("malaikat bench — API timing")
	if err := timeChatAPI(*url+"/v1/chat/completions", "malaikat", *prompt, *maxTokens); err != nil {
		fmt.Fprintf(os.Stderr, "malaikat API: %v\n", err)
		fmt.Fprintln(os.Stderr, "(start server first: malaikat serve -config ...)")
	}

	if strings.TrimSpace(*ollamaModel) != "" {
		fmt.Println()
		if err := timeOllama(*ollamaModel, *prompt, *maxTokens); err != nil {
			fmt.Fprintf(os.Stderr, "ollama: %v\n", err)
		}
	}

	if *kernel {
		modelPath, err := requireModel(cfg)
		if err != nil {
			return err
		}
		inst, err := engine.EnsureROCm(false)
		if err != nil {
			return err
		}
		if inst.BenchExe == "" {
			return fmt.Errorf("llama-bench not in runtime")
		}
		profile := optimize.FromConfig(cfg)
		bargs := engine.BuildBenchArgs(modelPath, profile)
		fmt.Println()
		fmt.Println("llama-bench:", engine.FormatArgs(inst.BenchExe, bargs))
		cmd := engine.BenchCommand(inst, bargs, profile)
		return cmd.Run()
	}
	return nil
}

func timeChatAPI(endpoint, label, prompt string, maxTokens int) error {
	body := map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  maxTokens,
		"temperature": 0.0,
		"stream":      false,
	}
	raw, _ := json.Marshal(body)
	start := time.Now()
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	elapsed := time.Since(start)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", resp.Status, truncate(string(data), 300))
	}
	var parsed struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Timings struct {
			PredictedN         float64 `json:"predicted_n"`
			PredictedPerSecond float64 `json:"predicted_per_second"`
			PromptPerSecond    float64 `json:"prompt_per_second"`
		} `json:"timings"`
	}
	_ = json.Unmarshal(data, &parsed)
	comp := parsed.Usage.CompletionTokens
	if comp == 0 && parsed.Timings.PredictedN > 0 {
		comp = int(parsed.Timings.PredictedN)
	}
	tps := 0.0
	if elapsed.Seconds() > 0 && comp > 0 {
		tps = float64(comp) / elapsed.Seconds()
	}
	if parsed.Timings.PredictedPerSecond > 0 {
		fmt.Printf("%s: completion=%d wall=%.2fs ~%.1f tok/s (server tg=%.1f pp=%.1f)\n",
			label, comp, elapsed.Seconds(), tps,
			parsed.Timings.PredictedPerSecond, parsed.Timings.PromptPerSecond)
	} else {
		fmt.Printf("%s: completion=%d wall=%.2fs ~%.1f tok/s\n", label, comp, elapsed.Seconds(), tps)
	}
	return nil
}

func timeOllama(model, prompt string, maxTokens int) error {
	body := map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"num_predict": maxTokens,
			"temperature": 0.0,
		},
	}
	raw, _ := json.Marshal(body)
	start := time.Now()
	resp, err := http.Post("http://127.0.0.1:11434/api/generate", "application/json", bytes.NewReader(raw))
	if err != nil {
		// fall back to CLI
		return timeOllamaCLI(model, prompt)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	elapsed := time.Since(start)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", resp.Status, truncate(string(data), 300))
	}
	var parsed struct {
		EvalCount    int    `json:"eval_count"`
		EvalDuration int64  `json:"eval_duration"`
		Response     string `json:"response"`
	}
	_ = json.Unmarshal(data, &parsed)
	tps := 0.0
	if parsed.EvalDuration > 0 && parsed.EvalCount > 0 {
		tps = float64(parsed.EvalCount) / (float64(parsed.EvalDuration) / 1e9)
	} else if elapsed.Seconds() > 0 && parsed.EvalCount > 0 {
		tps = float64(parsed.EvalCount) / elapsed.Seconds()
	}
	fmt.Printf("ollama(%s): completion=%d wall=%.2fs ~%.1f tok/s (eval)\n",
		model, parsed.EvalCount, elapsed.Seconds(), tps)
	return nil
}

func timeOllamaCLI(model, prompt string) error {
	start := time.Now()
	cmd := exec.Command("ollama", "run", model, prompt)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return fmt.Errorf("%w: %s", err, truncate(string(out), 200))
	}
	fmt.Printf("ollama-cli(%s): wall=%.2fs (no token stats)\n", model, elapsed.Seconds())
	return nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
