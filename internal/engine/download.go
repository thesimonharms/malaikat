package engine

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	githubAPI   = "https://api.github.com/repos/lemonade-sdk/llamacpp-rocm/releases/latest"
	assetNeedle = "windows-rocm-gfx1151-x64.zip"
	userAgent   = "malaikat/0.1 (AMD Strix Halo; Windows ROCm)"
)

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// EnsureROCm installs the ROCm runtime: prefer a working local install, then
// the zip embedded in all-in-one builds, then download from GitHub.
// With force=true, try a network update first (fall back to embedded).
func EnsureROCm(force bool) (Install, error) {
	dir, err := installRoot()
	if err != nil {
		return Install{}, err
	}

	current, _ := ReadManifest(dir)
	if !force && current.Backend == "rocm" && exeExists(current) {
		return current, nil
	}

	if force {
		inst, err := downloadROCm(dir, current, true)
		if err == nil {
			return inst, nil
		}
		if HasEmbeddedROCm() {
			fmt.Fprintf(os.Stderr, "warning: download failed (%v); using embedded runtime\n", err)
			return installEmbedded(dir)
		}
		return Install{}, err
	}

	if HasEmbeddedROCm() {
		inst, err := installEmbedded(dir)
		if err == nil {
			return inst, nil
		}
		fmt.Fprintf(os.Stderr, "warning: embedded runtime extract failed: %v\n", err)
	}
	return downloadROCm(dir, current, false)
}

func installEmbedded(dir string) (Install, error) {
	tag, err := extractEmbeddedROCm(dir)
	if err != nil {
		return Install{}, err
	}
	extractDir := filepath.Join(dir, tag+"-rocm-gfx1151")
	inst := Install{
		Tag:     tag,
		Dir:     extractDir,
		Backend: "rocm",
		Fetched: time.Now().UTC(),
	}
	if err := resolveBins(&inst); err != nil {
		return Install{}, err
	}
	if err := WriteManifest(dir, inst); err != nil {
		return Install{}, err
	}
	fmt.Printf("Installed bundled llama.cpp ROCm %s → %s\n", inst.Tag, inst.Dir)
	return inst, nil
}

func downloadROCm(dir string, current Install, force bool) (Install, error) {
	rel, err := latestRelease()
	if err != nil {
		if current.Tag != "" && !force && exeExists(current) {
			return current, nil
		}
		if HasEmbeddedROCm() {
			return installEmbedded(dir)
		}
		return Install{}, err
	}

	assetURL, assetName, err := findROCmAsset(rel)
	if err != nil {
		return Install{}, err
	}

	if !force && current.Tag == rel.TagName && current.Backend == "rocm" && exeExists(current) {
		return current, nil
	}

	fmt.Printf("Downloading llama.cpp ROCm %s (%s)...\n", rel.TagName, assetName)
	zipPath := filepath.Join(dir, assetName)
	if err := downloadFile(assetURL, zipPath); err != nil {
		if HasEmbeddedROCm() {
			fmt.Fprintf(os.Stderr, "warning: download failed (%v); using embedded runtime\n", err)
			return installEmbedded(dir)
		}
		return Install{}, err
	}
	defer os.Remove(zipPath)

	extractDir := filepath.Join(dir, rel.TagName+"-rocm-gfx1151")
	if err := os.RemoveAll(extractDir); err != nil {
		return Install{}, err
	}
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return Install{}, err
	}
	if err := unzip(zipPath, extractDir); err != nil {
		return Install{}, err
	}

	inst := Install{
		Tag:     rel.TagName,
		Dir:     extractDir,
		Backend: "rocm",
		Fetched: time.Now().UTC(),
	}
	if err := resolveBins(&inst); err != nil {
		return Install{}, err
	}
	if err := WriteManifest(dir, inst); err != nil {
		return Install{}, err
	}
	fmt.Printf("Installed llama.cpp ROCm %s → %s\n", inst.Tag, inst.Dir)
	return inst, nil
}

func latestRelease() (ghRelease, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI, nil)
	if err != nil {
		return ghRelease{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ghRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ghRelease{}, fmt.Errorf("github releases: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return ghRelease{}, err
	}
	if rel.TagName == "" {
		return ghRelease{}, fmt.Errorf("github releases: empty tag")
	}
	return rel, nil
}

func findROCmAsset(rel ghRelease) (url, name string, err error) {
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, assetNeedle) {
			return a.BrowserDownloadURL, a.Name, nil
		}
	}
	return "", "", fmt.Errorf("no %s asset in release %s", assetNeedle, rel.TagName)
}

func downloadFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	var written int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if written%(64*1024*1024) < int64(n) {
				fmt.Printf("\r  %d MB", written/(1024*1024))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	fmt.Printf("\r  %d MB done\n", written/(1024*1024))
	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		cleanDest := filepath.Clean(dest) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) && filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("illegal zip path: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
