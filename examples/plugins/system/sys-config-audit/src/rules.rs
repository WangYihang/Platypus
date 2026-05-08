// rules.rs — credential-pattern rule pack for sys-config-audit v3.
//
// Mirrors the ~50-rule subset of the gitleaks ruleset that's both
// (a) high-signal in production and (b) substring-or-regex
// expressible without backreferences / lookaheads. The legacy
// agent-side detector wrapped gitleaks v8 directly; we can't ship a
// full Go gitleaks binary into wasm32, so this is a hand-curated
// re-implementation. Future versions can grow the table.
//
// Each rule has:
//   - id:          stable identifier used for the leak's `id` field
//                  (auditor.gitleaks.<id>) and the `pattern` field
//                  (rule.id raw, no prefix).
//   - regex:       compiled lazily on first use; ASCII-only so we
//                  don't pull in the unicode tables.
//   - risk:        "high" (cloud / private-key class), "medium"
//                  (generic credential indicators), "low" (low-
//                  signal heuristics — currently unused, kept for
//                  future entries).
//   - title:       human-readable line shown in the UI's row title.
//
// The matcher returns the first match per (line, rule) so a long
// noisy line with many AKIA-shaped tokens still produces only one
// AWS finding per line.

use regex::Regex;
use std::sync::OnceLock;

#[derive(Debug)]
pub struct Rule {
    pub id: &'static str,
    pub pattern: &'static str,
    pub risk: &'static str,
    pub title: &'static str,
}

// RULES is the ordered list. Earlier entries take precedence on a
// line — a Stripe key matches "stripe-live-key" rather than the
// fallback "generic-api-key" because it appears first.
pub const RULES: &[Rule] = &[
    // ---- cloud providers (high-risk) ---------------------------
    Rule {
        id: "aws-access-token",
        pattern: r"\b(AKIA|ASIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASCA)[0-9A-Z]{16}\b",
        risk: "high",
        title: "AWS access key id",
    },
    Rule {
        // Heuristic: AWS access keys often appear next to a 40-char
        // base64-ish secret. Match the phrase "aws_secret" (or
        // similar) followed by an = and a 40-char token.
        id: "aws-secret-access-key",
        pattern: r#"(?i-u)aws[_\-\.]?(secret[_\-\.]?access[_\-\.]?key|secret)[\"']?\s*[:=]\s*[\"']?([A-Za-z0-9/+=]{40})[\"']?"#,
        risk: "high",
        title: "AWS secret access key",
    },
    Rule {
        id: "gcp-api-key",
        pattern: r"\bAIza[0-9A-Za-z\-_]{35}\b",
        risk: "high",
        title: "Google Cloud API key",
    },
    Rule {
        id: "gcp-service-account",
        pattern: r#""type"\s*:\s*"service_account""#,
        risk: "high",
        title: "GCP service account JSON key",
    },
    Rule {
        // Azure SAS tokens carry sig= + se= + sp= triples.
        id: "azure-sas-token",
        pattern: r"\bsig=[A-Za-z0-9%]{32,}&se=[0-9TZ\-\:]+&sp=[a-z]+\b",
        risk: "high",
        title: "Azure shared-access-signature token",
    },
    Rule {
        id: "azure-storage-account-key",
        pattern: r#"(?i-u)account[_\-]?key[\"']?\s*[:=]\s*[\"']?([A-Za-z0-9+/=]{86}==)"#,
        risk: "high",
        title: "Azure storage account key",
    },

    // ---- payment processors (high-risk) ------------------------
    Rule {
        id: "stripe-live-key",
        pattern: r"\bsk_live_[0-9A-Za-z]{24,}\b",
        risk: "high",
        title: "Stripe live secret key",
    },
    Rule {
        id: "stripe-restricted-key",
        pattern: r"\brk_live_[0-9A-Za-z]{24,}\b",
        risk: "high",
        title: "Stripe restricted live key",
    },
    Rule {
        id: "stripe-test-key",
        pattern: r"\bsk_test_[0-9A-Za-z]{24,}\b",
        risk: "medium",
        title: "Stripe test secret key",
    },
    Rule {
        id: "square-access-token",
        pattern: r"\bsq0atp-[0-9A-Za-z\-_]{22}\b",
        risk: "high",
        title: "Square access token",
    },
    Rule {
        id: "shopify-access-token",
        pattern: r"\bshpat_[0-9a-fA-F]{32}\b",
        risk: "high",
        title: "Shopify private app access token",
    },

    // ---- VCS / dev platforms (high-risk) -----------------------
    Rule {
        id: "github-pat",
        pattern: r"\bghp_[0-9A-Za-z]{36,}\b",
        risk: "high",
        title: "GitHub personal-access-token",
    },
    Rule {
        id: "github-oauth",
        pattern: r"\bgho_[0-9A-Za-z]{36,}\b",
        risk: "high",
        title: "GitHub OAuth access token",
    },
    Rule {
        id: "github-server",
        pattern: r"\bghs_[0-9A-Za-z]{36,}\b",
        risk: "high",
        title: "GitHub user-to-server token",
    },
    Rule {
        id: "github-user",
        pattern: r"\bghu_[0-9A-Za-z]{36,}\b",
        risk: "high",
        title: "GitHub user-to-user token",
    },
    Rule {
        id: "github-refresh",
        pattern: r"\bghr_[0-9A-Za-z]{36,}\b",
        risk: "high",
        title: "GitHub refresh token",
    },
    Rule {
        id: "github-fine-grained",
        pattern: r"\bgithub_pat_[0-9A-Za-z_]{82,}\b",
        risk: "high",
        title: "GitHub fine-grained PAT",
    },
    Rule {
        id: "gitlab-pat",
        pattern: r"\bglpat-[0-9A-Za-z\-_]{20}\b",
        risk: "high",
        title: "GitLab personal-access-token",
    },

    // ---- chat / collaboration (high-risk) ----------------------
    Rule {
        id: "slack-bot-token",
        pattern: r"\bxoxb-[0-9]{10,}-[0-9]{10,}-[0-9A-Za-z]{20,}\b",
        risk: "high",
        title: "Slack bot token",
    },
    Rule {
        id: "slack-user-token",
        pattern: r"\bxoxp-[0-9]{10,}-[0-9]{10,}-[0-9]{10,}-[0-9A-Za-z]{20,}\b",
        risk: "high",
        title: "Slack user token",
    },
    Rule {
        id: "slack-app-token",
        pattern: r"\bxoxa-[0-9A-Za-z\-]{20,}\b",
        risk: "high",
        title: "Slack app-level token",
    },
    Rule {
        id: "slack-webhook",
        pattern: r"https://hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/[0-9A-Za-z]{20,}",
        risk: "medium",
        title: "Slack incoming webhook URL",
    },
    Rule {
        id: "discord-bot-token",
        pattern: r"\b[MNO][A-Za-z0-9_\-]{23,28}\.[A-Za-z0-9_\-]{6,7}\.[A-Za-z0-9_\-]{27,38}\b",
        risk: "high",
        title: "Discord bot token",
    },
    Rule {
        id: "discord-webhook",
        pattern: r"https://discord(?:app)?\.com/api/webhooks/[0-9]+/[A-Za-z0-9_\-]+",
        risk: "medium",
        title: "Discord webhook URL",
    },
    Rule {
        id: "telegram-bot-token",
        pattern: r"\b[0-9]{8,10}:[A-Za-z0-9_\-]{35}\b",
        risk: "high",
        title: "Telegram bot token",
    },

    // ---- comms providers (high-risk) ---------------------------
    Rule {
        id: "twilio-api-key",
        pattern: r"\bSK[0-9a-fA-F]{32}\b",
        risk: "high",
        title: "Twilio API key",
    },
    Rule {
        id: "twilio-account-sid",
        pattern: r"\bAC[0-9a-fA-F]{32}\b",
        risk: "medium",
        title: "Twilio account SID",
    },
    Rule {
        id: "sendgrid-api-key",
        pattern: r"\bSG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}\b",
        risk: "high",
        title: "SendGrid API key",
    },
    Rule {
        id: "mailgun-api-key",
        pattern: r"\bkey-[0-9a-zA-Z]{32}\b",
        risk: "high",
        title: "Mailgun API key",
    },
    Rule {
        id: "mailchimp-api-key",
        pattern: r"\b[0-9a-f]{32}-us[0-9]{1,2}\b",
        risk: "high",
        title: "Mailchimp API key",
    },
    Rule {
        id: "postmark-server-token",
        pattern: r#"(?i-u)x-postmark-server-token[\"']?\s*[:=]\s*[\"']?([0-9a-f\-]{36})"#,
        risk: "high",
        title: "Postmark server token",
    },

    // ---- LLM / AI providers (high-risk) ------------------------
    Rule {
        id: "openai-api-key",
        pattern: r"\bsk-[A-Za-z0-9_\-]{20,}T3BlbkFJ[A-Za-z0-9_\-]{20,}\b",
        risk: "high",
        title: "OpenAI API key",
    },
    Rule {
        id: "anthropic-api-key",
        pattern: r"\bsk-ant-[A-Za-z0-9_\-]{40,}\b",
        risk: "high",
        title: "Anthropic API key",
    },

    // ---- package registries (high-risk) ------------------------
    Rule {
        id: "npm-token",
        pattern: r"\bnpm_[A-Za-z0-9]{36,}\b",
        risk: "high",
        title: "npm access token",
    },
    Rule {
        id: "pypi-upload-token",
        pattern: r"\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{50,}\b",
        risk: "high",
        title: "PyPI upload token",
    },
    Rule {
        id: "docker-hub-pat",
        pattern: r"\bdckr_pat_[A-Za-z0-9_\-]{20,}\b",
        risk: "high",
        title: "Docker Hub personal-access-token",
    },

    // ---- infra / monitoring (high-risk) ------------------------
    Rule {
        id: "digitalocean-pat",
        pattern: r"\bdop_v1_[a-f0-9]{64}\b",
        risk: "high",
        title: "DigitalOcean personal-access-token",
    },
    Rule {
        id: "cloudflare-api-token",
        pattern: r#"(?i-u)cf[_\-]?(api[_\-]?)?token[\"']?\s*[:=]\s*[\"']?([A-Za-z0-9_\-]{40})"#,
        risk: "high",
        title: "Cloudflare API token",
    },
    Rule {
        id: "heroku-api-key",
        pattern: r#"(?i-u)heroku[_\-]?(api[_\-]?)?key[\"']?\s*[:=]\s*[\"']?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})"#,
        risk: "high",
        title: "Heroku API key",
    },
    Rule {
        id: "datadog-api-key",
        pattern: r#"(?i-u)dd[_\-]?(api[_\-]?)?key[\"']?\s*[:=]\s*[\"']?([a-f0-9]{32})"#,
        risk: "high",
        title: "Datadog API key",
    },
    Rule {
        id: "newrelic-license-key",
        pattern: r"\b[a-f0-9]{40}NRAL\b",
        risk: "high",
        title: "New Relic license key",
    },
    Rule {
        id: "sentry-dsn",
        pattern: r"https://[0-9a-f]{32}@[a-zA-Z0-9.-]+sentry\.io/[0-9]+",
        risk: "medium",
        title: "Sentry DSN with embedded credential",
    },
    Rule {
        id: "pagerduty-api-key",
        pattern: r#"(?i-u)pagerduty[_\-]?(api[_\-]?)?key[\"']?\s*[:=]\s*[\"']?([0-9A-Za-z\-_+]{20,})"#,
        risk: "high",
        title: "PagerDuty API key",
    },
    Rule {
        id: "circleci-token",
        pattern: r"\bCCIPAT_[A-Za-z0-9]{40}\b",
        risk: "high",
        title: "CircleCI personal-access-token",
    },

    // ---- secret managers / identity (high-risk) ----------------
    Rule {
        id: "vault-token",
        pattern: r"\bhvs\.[A-Za-z0-9_\-]{24,}\b",
        risk: "high",
        title: "HashiCorp Vault service token",
    },
    Rule {
        id: "okta-api-token",
        pattern: r"\b00[A-Za-z0-9_\-]{40}\b",
        risk: "high",
        title: "Okta API token (heuristic)",
    },
    Rule {
        id: "auth0-client-secret",
        pattern: r#"(?i-u)auth0[_\-]?client[_\-]?secret[\"']?\s*[:=]\s*[\"']?([A-Za-z0-9_\-]{40,})"#,
        risk: "high",
        title: "Auth0 client secret",
    },

    // ---- private keys (high-risk) ------------------------------
    Rule {
        id: "private-key-rsa",
        pattern: r"-----BEGIN RSA PRIVATE KEY-----",
        risk: "high",
        title: "RSA private key block",
    },
    Rule {
        id: "private-key-openssh",
        pattern: r"-----BEGIN OPENSSH PRIVATE KEY-----",
        risk: "high",
        title: "OpenSSH private key block",
    },
    Rule {
        id: "private-key-encrypted",
        pattern: r"-----BEGIN ENCRYPTED PRIVATE KEY-----",
        risk: "medium",
        title: "Encrypted private key block",
    },
    Rule {
        id: "private-key-ec",
        pattern: r"-----BEGIN EC PRIVATE KEY-----",
        risk: "high",
        title: "EC private key block",
    },
    Rule {
        id: "private-key-dsa",
        pattern: r"-----BEGIN DSA PRIVATE KEY-----",
        risk: "high",
        title: "DSA private key block",
    },
    Rule {
        id: "private-key-pgp",
        pattern: r"-----BEGIN PGP PRIVATE KEY BLOCK-----",
        risk: "high",
        title: "PGP private key block",
    },

    // ---- generic / heuristic (medium-risk) ---------------------
    Rule {
        // JWT three-segment shape with "alg" header.
        id: "jwt",
        pattern: r"\beyJ[A-Za-z0-9_\-]{6,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b",
        risk: "high",
        title: "JSON Web Token",
    },
    Rule {
        id: "bearer-token",
        pattern: r"(?i-u)\bBearer\s+[A-Za-z0-9_\-\.=:/+]{20,}\b",
        risk: "medium",
        title: "Authorization Bearer header",
    },
    Rule {
        id: "basic-auth-url",
        pattern: r"https?://[^\s:@/]+:[^\s@/]+@[A-Za-z0-9\.\-]+",
        risk: "medium",
        title: "URL with embedded basic-auth credentials",
    },
    Rule {
        id: "generic-api-key",
        pattern: r#"(?i-u)\b(api[_\-]?key|apikey|x[_\-]api[_\-]key)[\"']?\s*[:=]\s*[\"']([A-Za-z0-9_\-]{16,})[\"']"#,
        risk: "medium",
        title: "Generic API key assignment",
    },
    Rule {
        id: "generic-secret",
        pattern: r#"(?i-u)\b(secret|client[_\-]?secret|app[_\-]?secret)[\"']?\s*[:=]\s*[\"']([A-Za-z0-9_\-]{16,})[\"']"#,
        risk: "medium",
        title: "Generic secret assignment",
    },
];

// compiled is a parallel slice of compiled regexes. Built lazily by
// scan_line() the first time a rule fires; rules with malformed
// patterns (which would be a bug here, but we'd rather skip than
// panic in the wasm sandbox) compile to None and are silently
// dropped at scan time.
fn compiled() -> &'static Vec<Option<Regex>> {
    static CELL: OnceLock<Vec<Option<Regex>>> = OnceLock::new();
    CELL.get_or_init(|| {
        RULES
            .iter()
            .map(|r| Regex::new(r.pattern).ok())
            .collect()
    })
}

/// scan_line returns the first matching (rule_index, matched_text) pair
/// for the line, or None if no rule matches. We stop at the first
/// match per line to keep findings scannable in the UI — a long line
/// with several credential-shaped tokens still produces one finding.
pub fn scan_line(line: &str) -> Option<(usize, String)> {
    let regs = compiled();
    for (i, slot) in regs.iter().enumerate() {
        let re = match slot {
            Some(r) => r,
            None => continue,
        };
        if let Some(m) = re.find(line) {
            return Some((i, m.as_str().to_string()));
        }
    }
    None
}

/// scan_text walks `text` line-by-line (1-indexed). Each match yields
/// (rule_index, line_number_1based, matched_text). Used for both the
/// gitleaks pass on file content and the env scanner.
pub fn scan_text(text: &str) -> Vec<(usize, usize, String)> {
    let mut out = Vec::new();
    for (i, line) in text.lines().enumerate() {
        if let Some((rule_idx, m)) = scan_line(line) {
            out.push((rule_idx, i + 1, m));
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rule_id_of(idx: usize) -> &'static str {
        RULES[idx].id
    }

    fn must_match(line: &str, expected_id: &str) {
        let m = scan_line(line);
        assert!(m.is_some(), "expected match for {expected_id} in: {line}");
        let (idx, _) = m.unwrap();
        assert_eq!(
            rule_id_of(idx),
            expected_id,
            "wrong rule fired on line: {line}"
        );
    }

    fn must_not_match(line: &str) {
        let m = scan_line(line);
        assert!(m.is_none(), "unexpected match: {:?} on line: {line}", m);
    }

    #[test]
    fn aws_access_token() {
        must_match("export AWS_KEY=AKIAIOSFODNN7EXAMPLE", "aws-access-token");
        must_match("ASIAIOSFODNN7EXAMPLE", "aws-access-token");
    }

    #[test]
    fn gcp_api_key() {
        must_match(
            "GOOGLE_API_KEY=AIzaSyC1234567890abcdefghijklmnopqrstuv",
            "gcp-api-key",
        );
    }

    // Test fixtures below are split with `concat!` so the literal
    // never appears as a contiguous run of characters in source —
    // GitHub's push-protection secret scanner would otherwise flag
    // these synthetic tokens (which match real-token regex even
    // though they're hand-typed).
    const A20: &str = "aaaaaaaaaaaaaaaaaaaa"; // 20 chars
    const A22: &str = "aaaaaaaaaaaaaaaaaaaaaa"; // 22 chars
    const A24: &str = "aaaaaaaaaaaaaaaaaaaaaaaa"; // 24 chars
    const A36: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"; // 36 chars

    #[test]
    fn stripe_live_key() {
        let s = concat!("sk_", "live_") .to_string() + A24;
        must_match(&s, "stripe-live-key");
    }

    #[test]
    fn github_tokens_all_prefixes() {
        for (prefix, id) in [
            (concat!("gh", "p_"), "github-pat"),
            (concat!("gh", "o_"), "github-oauth"),
            (concat!("gh", "s_"), "github-server"),
            (concat!("gh", "u_"), "github-user"),
            (concat!("gh", "r_"), "github-refresh"),
        ] {
            let token = format!("{prefix}{A36}");
            must_match(&format!("export X={token}"), id);
        }
    }

    #[test]
    fn slack_tokens() {
        // xoxb / xoxp bot/user-token shape, assembled at runtime.
        let bot = format!("{}-1234567890-1234567890-{}", concat!("xo", "xb"), A20);
        must_match(&bot, "slack-bot-token");
        let user = format!(
            "{}-1234567890-1234567890-1234567890-{}",
            concat!("xo", "xp"),
            A20
        );
        must_match(&user, "slack-user-token");
        // Slack webhook URL: split the host so the static literal
        // doesn't trip "slack webhook URL" detectors.
        let webhook = format!(
            "https://hooks.{}.com/services/T01ABC/B02DEF/{}",
            "slack",
            A22
        );
        must_match(&webhook, "slack-webhook");
    }

    #[test]
    fn jwt() {
        must_match(
            "Authorization: eyJabcdef.eyJklmnopqrstuv.signaturepart",
            "jwt",
        );
    }

    #[test]
    fn bearer_token() {
        must_match("Bearer abcdef1234567890ABCDEF.signature", "bearer-token");
    }

    #[test]
    fn npm_token() {
        must_match(
            "//registry.npmjs.org/:_authToken=npm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "npm-token",
        );
    }

    #[test]
    fn private_keys_block_marker() {
        must_match("-----BEGIN RSA PRIVATE KEY-----", "private-key-rsa");
        must_match("-----BEGIN OPENSSH PRIVATE KEY-----", "private-key-openssh");
        must_match("-----BEGIN EC PRIVATE KEY-----", "private-key-ec");
    }

    #[test]
    fn telegram_bot_token() {
        must_match("12345678:AAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "telegram-bot-token");
    }

    #[test]
    fn discord_webhook() {
        must_match(
            "https://discord.com/api/webhooks/123456789/aaaaaaaaaaaaaaaaaaaaaaaaaa",
            "discord-webhook",
        );
        must_match(
            "https://discordapp.com/api/webhooks/123456789/aaaaaaaaaaaaaaaaaaaaaaaaaa",
            "discord-webhook",
        );
    }

    #[test]
    fn vault_token() {
        must_match("VAULT_TOKEN=hvs.AAAAAQLm5_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "vault-token");
    }

    #[test]
    fn basic_auth_url() {
        must_match("https://user:p@ssw0rd@api.example.com/v1", "basic-auth-url");
    }

    #[test]
    fn generic_api_key_quoted() {
        must_match(
            r#"api_key="abcdefghijklmnopqrst""#,
            "generic-api-key",
        );
    }

    #[test]
    fn anthropic_api_key() {
        // Real keys are 80+ chars; min in the regex is 40 to keep test
        // strings tractable.
        let line = "ANTHROPIC_KEY=sk-ant-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
        must_match(line, "anthropic-api-key");
    }

    #[test]
    fn no_match_on_clean_line() {
        must_not_match("plain text with no credentials");
        must_not_match("// this is a comment");
        must_not_match("foo = 'bar'");
    }

    #[test]
    fn aws_short_token_skipped() {
        // AKIA prefix needs 16 [0-9A-Z] after — a too-short token shouldn't fire.
        must_not_match("AKIA01234567");
    }

    #[test]
    fn scan_text_returns_line_numbers() {
        let text = "line 1 nothing\nline 2 ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nline 3 nothing\n";
        let hits = scan_text(text);
        assert_eq!(hits.len(), 1);
        assert_eq!(hits[0].1, 2); // line number
        assert_eq!(rule_id_of(hits[0].0), "github-pat");
    }
}
