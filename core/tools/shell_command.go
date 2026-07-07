package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

type shellCommandPolicy struct {
	Allowed bool
	Reason  string
	Bases   []string
	Network bool
}

var dangerousShellCommandBases = map[string]string{
	"alias":     "shell mutation is not allowed",
	"bash":      "nested shell execution is not allowed",
	"chmod":     "permission changes are not allowed",
	"chown":     "ownership changes are not allowed",
	"dd":        "raw disk writes are not allowed",
	"eval":      "dynamic shell evaluation is not allowed",
	"exec":      "process replacement is not allowed",
	"fish":      "nested shell execution is not allowed",
	"mkfs":      "filesystem formatting is not allowed",
	"mount":     "mounting filesystems is not allowed",
	"mv":        "moving files is not allowed from bash",
	"open":      "opening GUI applications is not allowed",
	"osascript": "system scripting is not allowed",
	"pkill":     "process killing is not allowed",
	"reboot":    "system shutdown is not allowed",
	"rm":        "deleting files is not allowed from bash",
	"rmdir":     "deleting directories is not allowed from bash",
	"sh":        "nested shell execution is not allowed",
	"shutdown":  "system shutdown is not allowed",
	"source":    "loading external shell code is not allowed",
	"sudo":      "privilege escalation is not allowed",
	"su":        "privilege escalation is not allowed",
	"umount":    "unmounting filesystems is not allowed",
	"zsh":       "nested shell execution is not allowed",
	".":         "loading external shell code is not allowed",
}

var networkShellCommandBases = map[string]struct{}{
	"curl":   {},
	"curlie": {},
	"http":   {},
	"https":  {},
	"httpie": {},
	"wget":   {},
	"xh":     {},
}

var blockedShellCommandBases = map[string]string{
	"aria2c":      "download accelerators are not allowed",
	"axel":        "download accelerators are not allowed",
	"chrome":      "browser commands are not allowed",
	"firefox":     "browser commands are not allowed",
	"links":       "interactive browser commands are not allowed",
	"lynx":        "interactive browser commands are not allowed",
	"nc":          "raw network connections are not allowed",
	"netcat":      "raw network connections are not allowed",
	"safari":      "browser commands are not allowed",
	"telnet":      "raw network connections are not allowed",
	"w3m":         "interactive browser commands are not allowed",
	"http-prompt": "interactive network commands are not allowed",
}

var shellControlTokens = map[string]struct{}{
	"|": {}, "&&": {}, "||": {}, ";": {},
}

func classifyShellCommand(command string) shellCommandPolicy {
	segments, err := splitShellSegments(command)
	if err != nil {
		return shellCommandPolicy{Reason: err.Error()}
	}
	if len(segments) == 0 {
		return shellCommandPolicy{Reason: "command cannot be empty"}
	}

	policy := shellCommandPolicy{Allowed: true}
	for _, segment := range segments {
		if len(segment) == 0 {
			return shellCommandPolicy{Reason: "empty command segment is not allowed"}
		}
		base := commandBase(segment[0])
		if base == "" {
			return shellCommandPolicy{Reason: "command base is empty"}
		}
		if isShellAssignment(segment[0]) {
			return shellCommandPolicy{Reason: "environment assignments before commands are not allowed"}
		}
		for _, token := range segment {
			if reason := validateSandboxToken(token); reason != "" {
				return shellCommandPolicy{Reason: reason}
			}
		}
		policy.Bases = append(policy.Bases, base)

		if reason, blocked := dangerousShellCommandBases[base]; blocked {
			return shellCommandPolicy{Reason: fmt.Sprintf("%s: %s", base, reason)}
		}
		if reason, blocked := blockedShellCommandBases[base]; blocked {
			return shellCommandPolicy{Reason: fmt.Sprintf("%s: %s", base, reason)}
		}
		if _, network := networkShellCommandBases[base]; network {
			if reason := validateNetworkCommand(segment); reason != "" {
				return shellCommandPolicy{Reason: fmt.Sprintf("%s: %s", base, reason)}
			}
			policy.Network = true
		}
	}
	return policy
}

func splitShellSegments(command string) ([][]string, error) {
	tokens, err := shellTokens(command)
	if err != nil {
		return nil, err
	}

	var segments [][]string
	var current []string
	for _, token := range tokens {
		if _, control := shellControlTokens[token]; control {
			if len(current) == 0 {
				return nil, fmt.Errorf("empty command before %q is not allowed", token)
			}
			segments = append(segments, current)
			current = nil
			continue
		}
		current = append(current, token)
	}
	if len(current) == 0 && len(tokens) > 0 {
		return nil, fmt.Errorf("empty trailing command segment is not allowed")
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments, nil
}

func shellTokens(command string) ([]string, error) {
	var tokens []string
	for i := 0; i < len(command); {
		r := rune(command[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if command[i] == '#' {
			break
		}
		if strings.HasPrefix(command[i:], "&&") || strings.HasPrefix(command[i:], "||") {
			tokens = append(tokens, command[i:i+2])
			i += 2
			continue
		}
		switch command[i] {
		case '|', ';':
			tokens = append(tokens, command[i:i+1])
			i++
			continue
		case '<', '>', '`', '$', '&', '(', ')', '{', '}':
			return nil, fmt.Errorf("unsupported shell syntax %q is not allowed", command[i:i+1])
		}

		token, next, err := scanShellWord(command, i)
		if err != nil {
			return nil, err
		}
		if token != "" {
			tokens = append(tokens, token)
		}
		i = next
	}
	return tokens, nil
}

func scanShellWord(command string, start int) (string, int, error) {
	var b strings.Builder
	for i := start; i < len(command); i++ {
		ch := command[i]
		if unicode.IsSpace(rune(ch)) || strings.ContainsRune("|;&<>(){}", rune(ch)) {
			return b.String(), i, nil
		}
		switch ch {
		case '\\':
			if i+1 >= len(command) {
				return "", 0, fmt.Errorf("trailing escape is not allowed")
			}
			i++
			b.WriteByte(command[i])
		case '\'', '"':
			next, err := scanQuotedShellWord(command, i, ch, &b)
			if err != nil {
				return "", 0, err
			}
			i = next
		case '`', '$':
			return "", 0, fmt.Errorf("unsupported shell syntax %q is not allowed", string(ch))
		default:
			b.WriteByte(ch)
		}
	}
	return b.String(), len(command), nil
}

func scanQuotedShellWord(command string, start int, quote byte, b *strings.Builder) (int, error) {
	for i := start + 1; i < len(command); i++ {
		ch := command[i]
		if ch == quote {
			return i, nil
		}
		if quote == '"' && (ch == '$' || ch == '`') {
			return 0, fmt.Errorf("substitution inside quotes is not allowed")
		}
		if quote == '"' && ch == '\\' {
			if i+1 >= len(command) {
				return 0, fmt.Errorf("trailing escape in quotes is not allowed")
			}
			i++
			b.WriteByte(command[i])
			continue
		}
		b.WriteByte(ch)
	}
	return 0, fmt.Errorf("unterminated quoted string is not allowed")
}

func commandBase(token string) string {
	base := filepath.Base(strings.TrimSpace(token))
	return strings.ToLower(base)
}

func isShellAssignment(token string) bool {
	name, _, ok := strings.Cut(token, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func validateSandboxToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" || isURLArg(token) || strings.HasPrefix(token, "-") {
		return ""
	}
	if strings.HasPrefix(token, "~") {
		return "home-directory path expansion is not allowed"
	}
	if filepath.IsAbs(token) {
		return "absolute filesystem paths are not allowed"
	}
	clean := filepath.Clean(token)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "path traversal outside the workspace is not allowed"
	}
	return ""
}

func validateNetworkCommand(argv []string) string {
	if len(argv) == 0 {
		return "empty network command"
	}

	base := commandBase(argv[0])
	switch base {
	case "curl", "curlie":
		return validateCurlLikeCommand(argv)
	case "wget":
		return validateWgetCommand(argv)
	case "http", "https", "httpie", "xh":
		return validateHTTPieLikeCommand(argv)
	default:
		return "network command is not allowed"
	}
}

func validateCurlLikeCommand(argv []string) string {
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			continue
		}
		if isURLArg(arg) {
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return "only URL arguments and safe read-only flags are allowed"
		}
		if strings.HasPrefix(arg, "--") {
			name, hasValue := splitLongFlag(arg)
			switch name {
			case "--get", "--head", "--include", "--location", "--max-time", "--connect-timeout",
				"--retry", "--retry-delay", "--silent", "--show-error", "--user-agent",
				"--compressed", "--fail", "--fail-with-body", "--ipv4", "--ipv6", "--http1.1",
				"--http2", "--request":
				if name == "--request" && hasValue {
					_, value, _ := strings.Cut(arg, "=")
					if !isReadOnlyHTTPMethod(value) {
						return "only GET and HEAD requests are allowed"
					}
					continue
				}
				if hasValue {
					continue
				}
				if flagNeedsValue(name) {
					i++
					if i >= len(argv) {
						return "network flag requires a value"
					}
					if name == "--request" && !isReadOnlyHTTPMethod(argv[i]) {
						return "only GET and HEAD requests are allowed"
					}
				}
			default:
				return "unsafe curl flag is not allowed"
			}
			continue
		}
		if reason := validateShortNetworkFlags(arg); reason != "" {
			return reason
		}
	}
	return ""
}

func validateWgetCommand(argv []string) string {
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if isURLArg(arg) {
			continue
		}
		switch {
		case arg == "-q" || arg == "-S" || arg == "--quiet" || arg == "--server-response":
		case arg == "-O" || arg == "--output-document":
			i++
			if i >= len(argv) || argv[i] != "-" {
				return "wget output must be stdout with -O -"
			}
		case strings.HasPrefix(arg, "-O"):
			if arg != "-O-" {
				return "wget output must be stdout with -O-"
			}
		case strings.HasPrefix(arg, "--output-document="):
			if strings.TrimPrefix(arg, "--output-document=") != "-" {
				return "wget output must be stdout"
			}
		default:
			return "unsafe wget flag is not allowed"
		}
	}
	return ""
}

func validateHTTPieLikeCommand(argv []string) string {
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if isURLArg(arg) {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "--headers", "--body", "--verbose", "--follow", "--check-status",
				"--ignore-stdin", "--timeout", "--print":
				if arg == "--timeout" || arg == "--print" {
					i++
					if i >= len(argv) {
						return "network flag requires a value"
					}
				}
			default:
				return "unsafe HTTP client flag is not allowed"
			}
			continue
		}
		if strings.Contains(arg, "=") {
			return "HTTP request body/query mutations are not allowed"
		}
	}
	return ""
}

func splitLongFlag(arg string) (string, bool) {
	if name, _, ok := strings.Cut(arg, "="); ok {
		return name, true
	}
	return arg, false
}

func flagNeedsValue(name string) bool {
	switch name {
	case "--max-time", "--connect-timeout", "--retry", "--retry-delay", "--user-agent", "--request":
		return true
	default:
		return false
	}
}

func validateShortNetworkFlags(arg string) string {
	for _, ch := range strings.TrimPrefix(arg, "-") {
		switch ch {
		case 'G', 'I', 'i', 'L', 's', 'S', 'f', 'k':
		case 'A', 'm':
			return ""
		case 'X':
			method := strings.TrimSpace(strings.TrimPrefix(arg, "-X"))
			if method == "" {
				return "curl -X must use an attached GET or HEAD method"
			}
			if !isReadOnlyHTTPMethod(method) {
				return "only GET and HEAD requests are allowed"
			}
			return ""
		default:
			return "unsafe curl short flag is not allowed"
		}
	}
	return ""
}

func isURLArg(arg string) bool {
	lower := strings.ToLower(strings.TrimSpace(arg))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isReadOnlyHTTPMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return true
	default:
		return false
	}
}
