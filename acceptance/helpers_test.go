package acceptance_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const userAgent = "gorge-acceptance-test/1.0"

func buildBinary(dest string) error {
	// The test runs from acceptance/, but go build needs the module root
	cmd := exec.Command("go", "build", "-o", dest, ".")
	cmd.Stderr = os.Stderr
	cmd.Dir = ".."
	return cmd.Run()
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}

func startServer(binary, modulesDir string, port int, extraArgs ...string) (*exec.Cmd, error) {
	args := []string{
		"serve",
		"--dev",
		"--modulesdir", modulesDir,
		"--port", fmt.Sprintf("%d", port),
		"--bind", "127.0.0.1",
	}
	args = append(args, extraArgs...)
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func waitForReady(port string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	base := "http://127.0.0.1:" + port
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", base+"/readyz", nil)
		req.Header.Set("User-Agent", userAgent)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready within %v", timeout)
}

func doRequest(method, urlStr string, body []byte) (*http.Response, []byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, urlStr, r)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	return resp, respBody, err
}

func generateTarball(name, version, author, license, summary string, tags []string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	metadata := map[string]interface{}{
		"name":         name,
		"version":      version,
		"author":       author,
		"license":      license,
		"summary":      summary,
		"source":       "https://github.com/" + author + "/" + name,
		"dependencies": []interface{}{},
	}
	if tags != nil {
		metadata["tags"] = tags
	}

	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	dir := name + "-" + version

	if err := tw.WriteHeader(&tar.Header{
		Name:     dir + "/metadata.json",
		Mode:     0644,
		Size:     int64(len(metaBytes)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(metaBytes); err != nil {
		return nil, err
	}

	readme := "# " + name + "\n\nThis is a test module.\n"
	if err := tw.WriteHeader(&tar.Header{
		Name:     dir + "/README.md",
		Mode:     0644,
		Size:     int64(len(readme)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(readme)); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeTarball(dir, name, version, author, license, summary string, tags []string) ([]byte, error) {
	data, err := generateTarball(name, version, author, license, summary, tags)
	if err != nil {
		return nil, err
	}
	slug := name + "-" + version
	moduleDir := filepath.Join(dir, name)
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(moduleDir, slug+".tar.gz"), data, 0644); err != nil {
		return nil, err
	}
	return data, nil
}

func baseURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}
