package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// AppSlugRegex enforces Contract-compliant ?app= slugs.
	AppSlugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

	// ErrUnsafePath indicates path traversal, drive/UNC prefix, or reserved NTFS name.
	ErrUnsafePath = errors.New("unsafe or disallowed relative path")
	// ErrSymlinkEscape indicates a symlink or NTFS junction points outside root.
	ErrSymlinkEscape = errors.New("resolved path escapes confined root")
)

var ntfsReservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

var sensitiveFieldRegex = regexp.MustCompile(`(?i)(authorization[=:\s"]+bearer\s+|bearer\s+|(?:x-local-token|access_token|refresh_token|password|rc_pass|secret|rclone_[a-z0-9_]+)[=:\s"]+)([^\s"',}]+)`)

// GenerateToken returns a URL-safe cryptographically random 256-bit token.
func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ValidateAppSlug validates ?app= query parameters against ^[a-z0-9][a-z0-9_-]{0,63}$.
func ValidateAppSlug(slug string) bool {
	return AppSlugRegex.MatchString(slug)
}

// IsLoopbackBind asserts that a listen address resolves exclusively to 127.0.0.1 or ::1.
func IsLoopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "*" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateLoopbackHost ensures Host matches 127.0.0.1:<port> or localhost:<port>.
func ValidateLoopbackHost(hostHeader string, expectedPort int) bool {
	if hostHeader == "" {
		return false
	}
	expected1 := fmt.Sprintf("127.0.0.1:%d", expectedPort)
	expected2 := fmt.Sprintf("localhost:%d", expectedPort)
	if expectedPort == 0 {
		h, _, err := net.SplitHostPort(hostHeader)
		if err != nil {
			h = hostHeader
		}
		return h == "127.0.0.1" || strings.EqualFold(h, "localhost")
	}
	return strings.EqualFold(hostHeader, expected1) || strings.EqualFold(hostHeader, expected2)
}

// ValidateOrigin checks whether an Origin header is forbidden.
// If strictNoOrigin is true (e.g., /mcp or /api/v1/sync), ANY Origin header returns false.
// Otherwise only an exact same-origin http://127.0.0.1:<port> or http://localhost:<port> passes.
func ValidateOrigin(origin string, expectedPort int, strictNoOrigin bool) bool {
	if origin == "" {
		return true
	}
	if strictNoOrigin {
		return false
	}
	if expectedPort == 0 {
		return strings.HasPrefix(origin, "http://127.0.0.1:") || strings.HasPrefix(origin, "http://localhost:")
	}
	return origin == fmt.Sprintf("http://127.0.0.1:%d", expectedPort) ||
		origin == fmt.Sprintf("http://localhost:%d", expectedPort)
}

// ExtractBearerToken reads Authorization: Bearer <tok> or X-Local-Token: <tok>.
func ExtractBearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	if tok := strings.TrimSpace(r.Header.Get("X-Local-Token")); tok != "" {
		return tok
	}
	return ""
}

// ConstantTimeTokenMatch compares candidate against allowed valid tokens.
func ConstantTimeTokenMatch(candidate string, validTokens ...string) bool {
	if candidate == "" {
		return false
	}
	candBytes := []byte(candidate)
	matched := false
	for _, tok := range validTokens {
		if tok == "" {
			continue
		}
		if subtle.ConstantTimeCompare(candBytes, []byte(tok)) == 1 {
			matched = true
		}
	}
	return matched
}

// SanitizeRelPath validates a relative filename or subpath without touching disk.
// Blocks .., drive prefixes (C:), UNC paths (\\), leading slashes, null bytes,
// control characters, trailing dots/spaces on segments, and NTFS reserved names.
func SanitizeRelPath(raw string) (string, error) {
	if raw == "" || strings.ContainsRune(raw, 0) {
		return "", ErrUnsafePath
	}
	for _, r := range raw {
		if r < 32 {
			return "", ErrUnsafePath
		}
	}
	if strings.HasPrefix(raw, `\\`) || strings.HasPrefix(raw, `//`) ||
		strings.HasPrefix(raw, `/`) || strings.HasPrefix(raw, `\`) {
		return "", ErrUnsafePath
	}
	if len(raw) >= 2 && ((raw[0] >= 'a' && raw[0] <= 'z') || (raw[0] >= 'A' && raw[0] <= 'Z')) && raw[1] == ':' {
		return "", ErrUnsafePath
	}

	normalized := strings.ReplaceAll(raw, `\`, `/`)
	parts := strings.Split(normalized, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", ErrUnsafePath
		}
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", ErrUnsafePath
		}
		if strings.ContainsAny(part, `<>:"|?*`) {
			return "", ErrUnsafePath
		}
		stem := part
		if dot := strings.IndexByte(stem, '.'); dot >= 0 {
			stem = stem[:dot]
		}
		if _, reserved := ntfsReservedNames[strings.ToUpper(stem)]; reserved {
			return "", ErrUnsafePath
		}
		cleanParts = append(cleanParts, part)
	}
	return strings.Join(cleanParts, "/"), nil
}

// ResolveConfinedPath ensures relPath resolves strictly inside root, blocking
// directory traversal, NTFS reserved paths, and symlink/junction escapes.
func ResolveConfinedPath(root string, relPath string) (string, error) {
	cleanRel, err := SanitizeRelPath(relPath)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if evalRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = evalRoot
	}

	candidate := filepath.Join(absRoot, filepath.FromSlash(cleanRel))
	if !isWithinRoot(absRoot, candidate) {
		return "", ErrSymlinkEscape
	}

	// Verify deepest existing ancestor does not resolve outside absRoot via symlink/junction.
	cur := candidate
	for {
		info, lerr := os.Lstat(cur)
		if lerr == nil {
			resolved, rerr := filepath.EvalSymlinks(cur)
			if rerr != nil {
				return "", ErrSymlinkEscape
			}
			if !isWithinRoot(absRoot, resolved) {
				return "", ErrSymlinkEscape
			}
			// Also block raw symlink leaf directly.
			if info.Mode()&os.ModeSymlink != 0 && !isWithinRoot(absRoot, resolved) {
				return "", ErrSymlinkEscape
			}
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return candidate, nil
}

func isWithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// HasFreeSpaceHeadroom enforces free_disk_bytes > incoming_bytes * 1.2 + 2 GiB.
func HasFreeSpaceHeadroom(targetDir string, incomingBytes int64) (bool, uint64, error) {
	const twoGiB uint64 = 2 * 1024 * 1024 * 1024
	avail, err := AvailableBytes(targetDir)
	if err != nil {
		return false, 0, err
	}
	if incomingBytes < 0 {
		incomingBytes = 0
	}
	required := uint64(float64(incomingBytes)*1.2) + twoGiB
	return avail > required, avail, nil
}

// RedactSecrets masks bearer tokens, OAuth tokens, passwords, and RCLONE_* values.
func RedactSecrets(input string) string {
	if input == "" {
		return ""
	}
	return sensitiveFieldRegex.ReplaceAllString(input, "${1}[REDACTED]")
}

// RedactMap deep-copies a string/any map with sensitive keys masked.
func RedactMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		switch typed := v.(type) {
		case map[string]any:
			out[k] = RedactMap(typed)
		case string:
			out[k] = RedactSecrets(typed)
		default:
			out[k] = v
		}
	}
	return out
}

func isSensitiveKey(k string) bool {
	lk := strings.ToLower(k)
	for _, needle := range []string{
		"authorization", "x-local-token", "token", "secret",
		"password", "rc_pass", "rclone_", "pin", "cookie",
	} {
		if strings.Contains(lk, needle) {
			return true
		}
	}
	return false
}
