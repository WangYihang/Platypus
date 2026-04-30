package config_audit

import (
	"fmt"
	"strings"
	"sync"

	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
)

// gitleaksDetector wraps a *detect.Detector behind a sync.Once. The
// detector parses the embedded gitleaks ruleset (~160 rules) at first
// use; once built it is read-only and safe to share across audits.
type gitleaksDetector struct {
	det *detect.Detector
	err error
}

var (
	detectorOnce sync.Once
	detectorInst gitleaksDetector
)

func sharedDetector() (*detect.Detector, error) {
	detectorOnce.Do(func() {
		d, err := detect.NewDetectorDefaultConfig()
		if err != nil {
			detectorInst.err = err
			return
		}
		// MaxTargetMegaBytes guards against scanning huge files (e.g.
		// a multi-GB error log that drifted into /etc). A per-source
		// cap is also enforced upstream by readCapped, but defense in
		// depth is cheap here.
		d.MaxTargetMegaBytes = 4
		detectorInst.det = d
	})
	return detectorInst.det, detectorInst.err
}

// ScanBytes runs the default gitleaks ruleset over data and returns
// our Leak shape. The category argument is stamped onto each leak so
// auditors can attribute hits back to themselves; the auditorID
// becomes the leak ID prefix and shows up in the UI's group header.
//
// Plaintext secrets never escape this function: every Leak.MatchRedacted
// is built via RedactSecret before being returned.
func ScanBytes(auditorID, category, location string, data []byte) ([]Leak, error) {
	d, err := sharedDetector()
	if err != nil {
		return nil, err
	}
	raw := d.DetectBytes(data)
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]Leak, 0, len(raw))
	for _, f := range raw {
		out = append(out, gitleaksToLeak(auditorID, category, location, f))
	}
	return out, nil
}

// ScanString is a convenience wrapper for callers holding a string —
// gitleaks itself routes both DetectBytes and DetectString through the
// same internal scanner.
func ScanString(auditorID, category, location, content string) ([]Leak, error) {
	return ScanBytes(auditorID, category, location, []byte(content))
}

func gitleaksToLeak(auditorID, category, location string, f report.Finding) Leak {
	loc := location
	if f.StartLine > 0 {
		loc = fmt.Sprintf("%s:%d", location, f.StartLine)
	}
	id := auditorID + ".gitleaks." + safeIDSegment(f.RuleID)
	title := f.Description
	if title == "" {
		title = "Possible secret detected"
	}
	return Leak{
		ID:            id,
		Category:      category,
		Risk:          riskForRule(f.RuleID),
		Title:         title,
		Location:      loc,
		MatchRedacted: RedactSecret(f.Secret),
		Pattern:       f.RuleID,
		Description:   "Detected by gitleaks rule " + f.RuleID + ". A credential-shaped string was found at this location and may be a real secret.",
		Remediation:   "If real, rotate the credential at its issuer, remove it from this location, and prefer a secret manager or environment-injected runtime value.",
	}
}

// safeIDSegment scrubs gitleaks rule ids into something safe to put
// inside our dotted leak id. Gitleaks ids are kebab-case ASCII so this
// is just a defensive measure against a future rule named oddly.
func safeIDSegment(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// riskForRule maps a gitleaks rule id onto our 4-level Risk scale.
// The default for an unknown rule is medium — gitleaks rules already
// passed entropy / context filters so a hit is rarely pure noise, but
// only the cloud-provider / private-key / JWT class rules are
// confident enough to deserve "high" without operator review.
func riskForRule(ruleID string) string {
	r := strings.ToLower(ruleID)
	switch {
	case containsAny(r,
		"aws-access-token", "aws-secret",
		"gcp-", "google-",
		"azure-",
		"private-key", "rsa-private-key", "openssh-private-key", "pgp-private-key", "ec-private-key",
		"jwt",
		"github-pat", "github-app-token", "github-fine-grained", "gitlab-pat",
		"slack-bot", "slack-app", "slack-user",
		"stripe-", "twilio", "mailgun", "sendgrid",
		"openai", "anthropic"):
		return RiskHigh
	case containsAny(r,
		"generic-api-key", "password", "secret", "token"):
		return RiskMedium
	default:
		return RiskMedium
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
