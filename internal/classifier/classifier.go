package classifier

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	WorkspaceWork      = 0
	WorkspaceCoding    = 1
	WorkspaceResearch  = 2
	WorkspaceTemporary = 3
	WorkspaceSensitive = 4
)

type Result struct {
	WorkspaceID int
	Category    string
	Tags        []string
	Sensitive   bool
}

var (
	reURL        = regexp.MustCompile(`^https?://\S+`)
	reEmail      = regexp.MustCompile(`^[\w._%+\-]+@[\w.\-]+\.[a-zA-Z]{2,}$`)
	reCreditCard = regexp.MustCompile(`\b(?:\d[ \-]*?){13,19}\b`)
	reAPIKey     = regexp.MustCompile(`\b(sk-[a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{36}|AKIA[A-Z0-9]{16})\b`)
	reJWT        = regexp.MustCompile(`\beyJ[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+\b`)
	rePassword   = regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey)\s*[=:]\s*\S{4,}`)

	reCodeKeywords = regexp.MustCompile(
		`(?i)\b(func|def|class|import|export|const|let|var|return|if|else|for|while|switch|case|struct|interface|fn|pub|use|mod)\b`,
	)
	reShebang  = regexp.MustCompile(`^#!`)
	reJSONStart = regexp.MustCompile(`^[\[{]`)
)

func Classify(text string) Result {
	t := strings.TrimSpace(text)

	// -- sensitive check first --
	if reCreditCard.MatchString(t) {
		return Result{WorkspaceSensitive, "sensitive", []string{"sensitive", "credit_card"}, true}
	}
	if reAPIKey.MatchString(t) {
		return Result{WorkspaceSensitive, "sensitive", []string{"sensitive", "api_key"}, true}
	}
	if reJWT.MatchString(t) {
		return Result{WorkspaceSensitive, "sensitive", []string{"sensitive", "jwt"}, true}
	}
	if rePassword.MatchString(t) {
		return Result{WorkspaceSensitive, "sensitive", []string{"sensitive", "password"}, true}
	}

	// -- URL --
	if reURL.MatchString(t) {
		return Result{WorkspaceResearch, "url", []string{"url"}, false}
	}

	// -- email --
	if reEmail.MatchString(t) {
		return Result{WorkspaceWork, "email", []string{"email"}, false}
	}

	// -- JSON --
	if reJSONStart.MatchString(t) {
		var js any
		if json.Unmarshal([]byte(t), &js) == nil {
			return Result{WorkspaceCoding, "json", []string{"code", "json"}, false}
		}
	}

	// -- shebang --
	if reShebang.MatchString(t) {
		return Result{WorkspaceCoding, "code", []string{"code", "script"}, false}
	}

	// -- code keywords heuristic --
	matches := reCodeKeywords.FindAllString(t, -1)
	if len(matches) >= 3 {
		return Result{WorkspaceCoding, "code", []string{"code"}, false}
	}

	// -- long text -> research --
	words := strings.Fields(t)
	if len(words) > 30 {
		return Result{WorkspaceResearch, "research", []string{"research", "text"}, false}
	}

	// -- default -> work --
	return Result{WorkspaceWork, "text", []string{"text"}, false}
}

func RedactHint(content string, charCount int) string {
	t := strings.TrimSpace(content)

	if reCreditCard.MatchString(t) {
		digits := regexp.MustCompile(`\D`).ReplaceAllString(t, "")
		if len(digits) >= 4 {
			return "[card] **** " + digits[len(digits)-4:]
		}
		return "[card] redacted"
	}
	if strings.HasPrefix(t, "eyJ") {
		return fmt.Sprintf("[jwt] %d chars", charCount)
	}
	if reAPIKey.MatchString(t) {
		if len(t) >= 8 {
			return "[key] " + t[:8] + "****"
		}
		return "[key] redacted"
	}
	m := rePassword.FindStringSubmatch(t)
	if len(m) >= 2 {
		val := rePassword.FindString(t)
		if len(val) > 12 {
			val = val[:12]
		}
		return "[secret] " + val + "****"
	}
	return fmt.Sprintf("[sensitive] %d chars", charCount)
}
