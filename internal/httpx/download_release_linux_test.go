package httpx

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReleaseKeepsPublishedAPKURLs(t *testing.T) {
	stack, bin := t.TempDir(), t.TempDir()
	write := func(path, value string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(stack, ".env"), "", 0o600)
	write(filepath.Join(bin, "curl"), "#!/bin/sh\nexit 0\n", 0o700)
	// The real scripts still create backups, lock, and switch releases. Only the
	// Docker commands and the readiness probe are replaced by fixtures.
	write(filepath.Join(bin, "docker"), `#!/bin/bash
set -eu
if [[ $1 == image && $2 == load ]]; then exit 0; fi
[[ $1 == compose && $2 == --env-file && $4 == -f ]]
case $6 in
    ps|stop) ;;
    up) if [[ $* != *--no-deps* ]]; then printf '%s\n%s\n' "$5" "$DEPLOY_APK_DIR" > "$MOUNT_RECORD"; fi ;;
    exec|run) printf 'backup fixture\n' ;;
    port) printf '127.0.0.1:8080\n' ;;
    *) exit 1 ;;
esac
`, 0o700)
	apply, err := filepath.Abs("../../deploy/apply-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(stack, "mounts")
	run := func(tag, incoming string) (string, error) {
		t.Helper()
		cmd := exec.Command("bash", apply, stack, tag, incoming)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "MOUNT_RECORD="+record)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	const version1, version2 = "1.0.0", "1.0.1"
	const content1, content2 = "first release contents\n", "second release contents\n"
	filename := func(version string) string { return "vanillapay-monitor-" + version + ".apk" }
	stage := func(tag, version, content string) string {
		t.Helper()
		dir := filepath.Join(stack, ".incoming-"+tag)
		for _, folder := range []string{"deploy", ""} {
			if err := os.MkdirAll(filepath.Join(dir, folder), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"release.tar.gz", "stack.tar.gz", "compose.yaml", "deploy/apply-release.sh",
			"deploy/backup.sh", "deploy/common.sh", "deploy/restore.sh"} {
			write(filepath.Join(dir, name), "", 0o600)
		}
		if version != "" {
			downloads := filepath.Join(dir, "downloads")
			if err := os.Mkdir(downloads, 0o700); err != nil {
				t.Fatal(err)
			}
			manifest := downloadManifest{Version: version, File: filename(version), SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(content))),
				Size: int64(len(content)), PublishedAt: "2026-11-09T00:00:00Z"}
			body, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(downloads, "latest.json"), string(body), 0o600)
			write(filepath.Join(downloads, manifest.File), content, 0o600)
		}
		return dir
	}
	success := func(tag, incoming string) {
		t.Helper()
		if out, err := run(tag, incoming); err != nil {
			t.Fatalf("apply %s: %v\n%s", tag, err, out)
		}
	}
	verify := func(version, content string, wantSecond bool) {
		t.Helper()
		mounts, err := os.ReadFile(record)
		if err != nil {
			t.Fatal(err)
		}
		paths := strings.Split(strings.TrimSpace(string(mounts)), "\n")
		if len(paths) != 2 || paths[1] != filepath.Join(stack, "downloads", "apks") {
			t.Fatalf("unexpected Compose sources %q", paths)
		}
		handler := downloadHandler(filepath.Join(filepath.Dir(paths[0]), "downloads"), paths[1])
		fetch := func(method, name, byteRange, body string, status int) *httptest.ResponseRecorder {
			t.Helper()
			req := httptest.NewRequest(method, "/downloads/"+name, nil)
			req.Header.Set("Range", byteRange)
			res := httptest.NewRecorder()
			handler(res, req)
			if res.Code != status || res.Body.String() != body {
				t.Fatalf("%s %s %s: status=%d body=%q", method, name, byteRange, res.Code, res.Body)
			}
			return res
		}
		manifestRes := httptest.NewRecorder()
		handler(manifestRes, httptest.NewRequest(http.MethodGet, "/downloads/latest.json", nil))
		var manifest downloadManifest
		if manifestRes.Code != http.StatusOK || json.Unmarshal(manifestRes.Body.Bytes(), &manifest) != nil ||
			manifest.Version != version || manifest.File != filename(version) ||
			manifest.Size != int64(len(content)) || manifest.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(content))) {
			t.Fatalf("unexpected manifest status=%d body=%q", manifestRes.Code, manifestRes.Body)
		}
		for _, item := range []struct{ version, content string }{{version1, content1}, {version2, content2}} {
			name := filename(item.version)
			if item.version == version2 && !wantSecond {
				fetch(http.MethodGet, name, "", "404 page not found\n", http.StatusNotFound)
				continue
			}
			full := fetch(http.MethodGet, name, "", item.content, http.StatusOK)
			if sha256.Sum256(full.Body.Bytes()) != sha256.Sum256([]byte(item.content)) {
				t.Fatalf("%s content hash changed", name)
			}
			head := fetch(http.MethodHead, name, "", "", http.StatusOK)
			if head.Header().Get("Content-Length") != strconv.Itoa(len(item.content)) {
				t.Fatalf("HEAD %s: headers=%v", name, head.Header())
			}
			part := fetch(http.MethodGet, name, "bytes=0-9", item.content[:10], http.StatusPartialContent)
			if part.Header().Get("Content-Range") != fmt.Sprintf("bytes 0-9/%d", len(item.content)) {
				t.Fatalf("Range %s: headers=%v", name, part.Header())
			}
		}
	}
	success("aaaaaaa", stage("aaaaaaa", version1, content1))
	verify(version1, content1, false)
	success("bbbbbbb", stage("bbbbbbb", version2, content2))
	verify(version2, content2, true)
	success("--rollback", "")
	verify(version1, content1, true)
	success("ccccccc", stage("ccccccc", "", ""))
	verify(version1, content1, true)
	published := filepath.Join(stack, "downloads", "apks", filename(version1))
	before, err := os.Stat(published)
	if err != nil {
		t.Fatal(err)
	}
	success("ddddddd", stage("ddddddd", version1, content1))
	if after, err := os.Stat(published); err != nil || !os.SameFile(before, after) {
		t.Fatalf("byte-identical publication replaced the original APK: %v", err)
	}
	verify(version1, content1, true)
	if out, err := run("eeeeeee", stage("eeeeeee", version1, "different contents\n")); err == nil || !strings.Contains(out, "already exists with different bytes") {
		t.Fatalf("expected conflicting APK rejection, got %v\n%s", err, out)
	}
	verify(version1, content1, true)
}
