// Package diagnostics retains bounded local logs for unattended network diagnosis.
package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const maxBytes = 4 * 1024 * 1024

var sink struct {
	sync.Mutex
	dir string
}

var credentials = regexp.MustCompile(`(?i)((?:device_?token|access_?token|refresh_?token|auth_?key|secret|password|authorization|token)\s*[=:]\s*)("[^"]*"|[^\s,;]+)`)
var querySecrets = regexp.MustCompile(`(?i)([?&](?:code|state|token|key|secret|authkey)=)[^&\s]+`)
var bearerSecrets = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;]+`)

func redact(s string) string {
	s = bearerSecrets.ReplaceAllString(s, "Bearer [REDACTED]")
	s = credentials.ReplaceAllString(s, "${1}[REDACTED]")
	return querySecrets.ReplaceAllString(s, "${1}[REDACTED]")
}

// Init uses the same isolated application directory as the client.
func Init(appDir string) error {
	dir := filepath.Join(appDir, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	sink.Lock()
	sink.dir = dir
	sink.Unlock()
	return nil
}

// Record never calls the standard logger, including on disk failures.
func Record(channel, message string) {
	sink.Lock()
	defer sink.Unlock()
	if sink.dir == "" {
		return
	}
	if channel != "network" {
		channel = "desktop"
	}
	line := []byte(time.Now().Format(time.RFC3339Nano) + " " + redact(strings.TrimSpace(message)) + "\n")
	if err := appendBounded(filepath.Join(sink.dir, channel+".log"), line, maxBytes); err != nil {
		fmt.Fprintf(os.Stderr, "diagnostic log write failed: %v\n", err)
	}
}

func appendBounded(path string, p []byte, limit int64) error {
	if int64(len(p)) > limit {
		p = append(p[:limit-1], '\n')
	}
	if st, err := os.Stat(path); err == nil && st.Size()+int64(len(p)) > limit {
		if err := os.Remove(path + ".1"); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(path, path+".1"); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(p)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Writer captures startup logs before the UI logger is installed.
type Writer struct{}

func (Writer) Write(p []byte) (int, error) {
	Record("desktop", string(p))
	return len(p), nil
}

// Tunnel keeps discovery diagnostics even when the UI suppresses verbose logs.
func Tunnel(message string) {
	lower := strings.ToLower(message)
	for _, tag := range []string{"magicsock:", "netcheck:", "derp", "portmap", "stun", "control: netinfo:", "linkchange:", "health("} {
		if strings.Contains(lower, tag) {
			Record("network", message)
			return
		}
	}
}
