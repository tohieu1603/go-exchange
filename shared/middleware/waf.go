package middleware

import (
	"bytes"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// WAF — minimal Web Application Firewall middleware. Mounted at the gateway,
// before route resolution, to filter cheap/known-bad requests early.
//
// Goals: defense-in-depth (the actual services validate inputs too).
// Non-goals: deep payload inspection, signature databases. For full WAF,
// front the gateway with Cloudflare/AWS WAF/ModSecurity in production.
//
// Rules are evaluated cheaply against URL path, query, and bounded body.
// On match → 403 with reason. Otherwise pass through.
//
// Body inspection skipped for >MaxBodyScan (default 64 KiB) to keep CPU bounded.

const (
	wafMaxBodyScan    = 64 * 1024
	wafMaxRequestBody = 5 * 1024 * 1024 // 5 MiB hard limit at gateway
)

// rulePattern is a labelled regex for matching+telemetry.
type rulePattern struct {
	Name  string
	RE    *regexp.Regexp
	OnURL bool // also match URL/query (in addition to body)
}

// Pre-compiled rules. Keep narrow to avoid false positives on legit JSON.
var (
	wafRulePathTraversal = rulePattern{
		Name:  "path_traversal",
		RE:    regexp.MustCompile(`(?:\.\./){2,}|/\.\./|\\\.\\\.\\`),
		OnURL: true,
	}
	wafRuleSQLi = rulePattern{
		Name: "sqli",
		RE: regexp.MustCompile(`(?i)(?:` +
			`union\s+(?:all\s+)?select|` +
			`select\s+.{1,200}\s+from\s+|` +
			`(?:'|"|\b)or\s+1\s*=\s*1\b|` +
			`(?:'|"|\b)and\s+1\s*=\s*1\b|` +
			`drop\s+table\s+|` +
			`exec\s*\(\s*xp_|` +
			`information_schema\.|` +
			`sleep\s*\(\s*\d+\s*\)|benchmark\s*\(` +
			`)`),
		OnURL: true,
	}
	wafRuleXSS = rulePattern{
		Name: "xss",
		RE: regexp.MustCompile(`(?i)(?:` +
			`<script\b|javascript:|onerror\s*=|onload\s*=|` +
			`<iframe\b|<svg\b[^>]*onload|` +
			`document\.cookie|window\.location` +
			`)`),
		OnURL: true,
	}
	wafRuleScannerPaths = rulePattern{
		Name: "scanner_path",
		RE: regexp.MustCompile(
			`/\.env(?:[/?]|$)|` +
				`/\.git(?:/|$)|` +
				`/\.aws/|` +
				`/wp-(?:admin|login|content)/|` +
				`/phpmyadmin/|` +
				`/xmlrpc\.php|` +
				`/server-status\b|` +
				`/cgi-bin/|` +
				`/etc/passwd`),
		OnURL: true,
	}
	wafRuleCmdInjection = rulePattern{
		Name: "cmd_injection",
		RE: regexp.MustCompile(
			"(?:;|\\|\\||&&|`)\\s*(?:cat|whoami|id|wget|curl|nc|bash|sh)\\b"),
		OnURL: true,
	}

	wafRules = []rulePattern{
		wafRulePathTraversal,
		wafRuleSQLi,
		wafRuleXSS,
		wafRuleScannerPaths,
		wafRuleCmdInjection,
	}
)

// WAF returns a Gin middleware that runs the curated rule set.
// `whitelistPaths` are exact paths that bypass body scanning (e.g. webhooks
// that accept arbitrary text). The URL itself is always scanned.
func WAF(whitelistPaths ...string) gin.HandlerFunc {
	whiteSet := make(map[string]bool, len(whitelistPaths))
	for _, p := range whitelistPaths {
		whiteSet[p] = true
	}
	return func(c *gin.Context) {
		// 1. Hard body size cap.
		if c.Request.ContentLength > wafMaxRequestBody {
			waf403(c, "body_too_large")
			return
		}

		// 2. URL + query scan — runs against every request.
		// Match BOTH raw and URL-decoded forms so attackers can't bypass via
		// "+" / "%20" / "%55NION" tricks.
		raw := c.Request.URL.Path + "?" + c.Request.URL.RawQuery
		decoded := raw
		if d, err := url.QueryUnescape(c.Request.URL.RawQuery); err == nil {
			decoded = c.Request.URL.Path + "?" + d
		}
		// Convert "+" → space inside the decoded form (form-encoding standard).
		decoded = strings.ReplaceAll(decoded, "+", " ")
		for _, r := range wafRules {
			if r.OnURL && (r.RE.MatchString(raw) || r.RE.MatchString(decoded)) {
				waf403(c, r.Name)
				return
			}
		}

		// 3. Body scan — bounded, only if Content-Length looks reasonable.
		if !whiteSet[c.Request.URL.Path] && c.Request.Body != nil &&
			c.Request.ContentLength > 0 && c.Request.ContentLength <= wafMaxBodyScan {
			body, err := io.ReadAll(c.Request.Body)
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
				bodyStr := string(body)
				for _, r := range wafRules {
					if r.RE.MatchString(bodyStr) {
						waf403(c, r.Name+"_body")
						return
					}
				}
			}
		}

		c.Next()
	}
}

func waf403(c *gin.Context, rule string) {
	// Generic message to the client; specific rule tag goes to logs.
	c.JSON(403, gin.H{
		"success": false,
		"message": "request blocked by security policy",
		"rule":    rule,
	})
	c.Abort()
}

// LooksLikeJSON — utility used by services that want light double-checking
// before parsing user-supplied strings.
func LooksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}
