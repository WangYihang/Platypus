package main

import "testing"

// ---- ScanLine / Rules table ---------------------------------------

func mustMatch(t *testing.T, line, expectedID string) {
	t.Helper()
	idx, _ := ScanLine(line)
	if idx < 0 {
		t.Fatalf("expected match for %q in: %q", expectedID, line)
	}
	if Rules[idx].ID != expectedID {
		t.Errorf("wrong rule fired on %q: got %q, want %q",
			line, Rules[idx].ID, expectedID)
	}
}

func mustNotMatch(t *testing.T, line string) {
	t.Helper()
	if idx, m := ScanLine(line); idx >= 0 {
		t.Errorf("unexpected match: rule=%q match=%q on line: %q",
			Rules[idx].ID, m, line)
	}
}

func TestRules_AWSAccessToken(t *testing.T) {
	mustMatch(t, "export AWS_KEY=AKIAIOSFODNN7EXAMPLE", "aws-access-token")
	mustMatch(t, "ASIAIOSFODNN7EXAMPLE", "aws-access-token")
}

func TestRules_GCP_APIKey(t *testing.T) {
	mustMatch(t,
		"GOOGLE_API_KEY=AIzaSyC1234567890abcdefghijklmnopqrstuv",
		"gcp-api-key")
}

// Test fixtures below assemble synthetic tokens at runtime via
// string concatenation so the literal value never appears as a
// contiguous run in source — GitHub's push-protection secret
// scanner would otherwise flag these hand-typed test tokens.
const (
	a20 = "aaaaaaaaaaaaaaaaaaaa"                         // 20 chars
	a22 = "aaaaaaaaaaaaaaaaaaaaaa"                       // 22 chars
	a24 = "aaaaaaaaaaaaaaaaaaaaaaaa"                     // 24 chars
	a36 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"         // 36 chars
)

func TestRules_StripeLiveKey(t *testing.T) {
	token := "sk_" + "live_" + a24
	mustMatch(t, token, "stripe-live-key")
}

func TestRules_GitHubTokens(t *testing.T) {
	cases := []struct{ prefix, id string }{
		{"gh" + "p_", "github-pat"},
		{"gh" + "o_", "github-oauth"},
		{"gh" + "s_", "github-server"},
		{"gh" + "u_", "github-user"},
		{"gh" + "r_", "github-refresh"},
	}
	for _, c := range cases {
		mustMatch(t, "X="+c.prefix+a36, c.id)
	}
}

func TestRules_SlackTokens(t *testing.T) {
	bot := "xo" + "xb-1234567890-1234567890-" + a20
	mustMatch(t, bot, "slack-bot-token")
	user := "xo" + "xp-1234567890-1234567890-1234567890-" + a20
	mustMatch(t, user, "slack-user-token")
	// Split host so the literal slack-webhook URL doesn't appear
	// contiguously in source.
	webhook := "https://hooks." + "slack.com" + "/services/T01ABC/B02DEF/" + a22
	mustMatch(t, webhook, "slack-webhook")
}

func TestRules_JWT(t *testing.T) {
	mustMatch(t,
		"Authorization: eyJabcdef.eyJklmnopqrstuv.signaturepart",
		"jwt")
}

func TestRules_BearerToken(t *testing.T) {
	mustMatch(t, "Bearer abcdef1234567890ABCDEF.signature", "bearer-token")
}

func TestRules_NPMToken(t *testing.T) {
	mustMatch(t,
		"//registry.npmjs.org/:_authToken=npm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"npm-token")
}

func TestRules_PrivateKeyMarkers(t *testing.T) {
	mustMatch(t, "-----BEGIN RSA PRIVATE KEY-----", "private-key-rsa")
	mustMatch(t, "-----BEGIN OPENSSH PRIVATE KEY-----", "private-key-openssh")
	mustMatch(t, "-----BEGIN EC PRIVATE KEY-----", "private-key-ec")
}

func TestRules_TelegramBotToken(t *testing.T) {
	mustMatch(t, "12345678:AAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "telegram-bot-token")
}

func TestRules_DiscordWebhook(t *testing.T) {
	mustMatch(t,
		"https://discord.com/api/webhooks/123456789/aaaaaaaaaaaaaaaaaaaaaaaaaa",
		"discord-webhook")
	mustMatch(t,
		"https://discordapp.com/api/webhooks/123456789/aaaaaaaaaaaaaaaaaaaaaaaaaa",
		"discord-webhook")
}

func TestRules_VaultToken(t *testing.T) {
	mustMatch(t, "VAULT_TOKEN=hvs.AAAAAQLm5_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "vault-token")
}

func TestRules_BasicAuthURL(t *testing.T) {
	mustMatch(t, "https://user:p@ssw0rd@api.example.com/v1", "basic-auth-url")
}

func TestRules_AnthropicAPIKey(t *testing.T) {
	mustMatch(t,
		"ANTHROPIC_KEY=sk-ant-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"anthropic-api-key")
}

func TestRules_GenericAPIKey(t *testing.T) {
	mustMatch(t, `api_key="abcdefghijklmnopqrst"`, "generic-api-key")
}

func TestRules_CleanLineNoMatch(t *testing.T) {
	mustNotMatch(t, "plain text with no credentials")
	mustNotMatch(t, "// this is a comment")
	mustNotMatch(t, "foo = 'bar'")
}

func TestRules_AWSShortTokenSkipped(t *testing.T) {
	mustNotMatch(t, "AKIA01234567")
}

func TestScanText_LineNumbers(t *testing.T) {
	text := "line 1 nothing\nline 2 ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nline 3 nothing\n"
	hits := ScanText(text)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	if hits[0].LineNo != 2 {
		t.Errorf("LineNo = %d, want 2", hits[0].LineNo)
	}
	if Rules[hits[0].RuleIdx].ID != "github-pat" {
		t.Errorf("rule = %q, want github-pat", Rules[hits[0].RuleIdx].ID)
	}
}

// ---- NormalizeZshHistory ------------------------------------------

func TestNormalizeZsh_StripsTimestamp(t *testing.T) {
	raw := ": 1700000000:0;ls -la\n: 1700000010:1;cd /tmp\nplain command\n"
	got := NormalizeZshHistory(raw)
	for _, want := range []string{"ls -la", "cd /tmp", "plain command"} {
		if !contains_string(got, want) {
			t.Errorf("expected %q in: %q", want, got)
		}
	}
	if contains_string(got, "1700000000") {
		t.Error("timestamp leaked into output")
	}
}

func TestNormalizeZsh_BashPassthrough(t *testing.T) {
	raw := "ls -la\ncd /tmp\n"
	if got := NormalizeZshHistory(raw); got != "ls -la\ncd /tmp\n" {
		t.Errorf("got %q", got)
	}
}

// ---- behavioural --------------------------------------------------

func TestBehavioural_MysqlInline(t *testing.T) {
	if !matchesMysqlInline("mysql -u root -psecret") {
		t.Error("expected match")
	}
	if !matchesMysqlInline("mysql -psecret -u root") {
		t.Error("expected match")
	}
	if matchesMysqlInline("mysql -u root -p") {
		t.Error("interactive -p should not match")
	}
	if matchesMysqlInline("ls -p /tmp") {
		t.Error("non-mysql -p should not match")
	}
}

func TestBehavioural_CurlBasicAuth(t *testing.T) {
	if !matchesCurlBasicAuth("curl -u user:pass https://api") {
		t.Error("expected match")
	}
	if !matchesCurlBasicAuth("curl --user user:pass https://api") {
		t.Error("expected match")
	}
	if matchesCurlBasicAuth("curl -u user: https://api") {
		t.Error("empty password should not match")
	}
	if matchesCurlBasicAuth("wget -u user:pass") {
		t.Error("non-curl should not match")
	}
}

func TestBehavioural_ExportSecret(t *testing.T) {
	if !matchesExportSecret("export AWS_SECRET_KEY=foo") {
		t.Error("expected match")
	}
	if !matchesExportSecret("export DB_PASSWORD=hunter2") {
		t.Error("expected match")
	}
	if !matchesExportSecret("export FOO_PWD=x") {
		t.Error("expected match")
	}
	if matchesExportSecret("export PATH=/usr/bin") {
		t.Error("non-credential suffix should not match")
	}
	if matchesExportSecret("FOO=bar") {
		t.Error("missing 'export' should not match")
	}
}

func TestMatchBehavioural_Routes(t *testing.T) {
	cases := []struct {
		line, id string
	}{
		{"mysql -u root -psecret", "mysql-inline-password"},
		{"curl -u admin:hunter2 https://x", "curl-basic-auth"},
		{"export GITHUB_TOKEN=abc", "export-secret"},
	}
	for _, c := range cases {
		b, ok := MatchBehavioural(c.line)
		if !ok {
			t.Errorf("%q: expected match", c.line)
			continue
		}
		if b.ID != c.id {
			t.Errorf("%q: got rule %q, want %q", c.line, b.ID, c.id)
		}
	}
	if _, ok := MatchBehavioural("ls -la"); ok {
		t.Error("clean line should not match")
	}
}

// ---- RedactCommandLine --------------------------------------------

func TestRedactCommandLine_Truncates(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	red := RedactCommandLine(long)
	if !contains_string(red, "…") {
		t.Errorf("expected ellipsis in %q", red)
	}
	if len(red) >= len(long) {
		t.Error("expected truncation")
	}
}

func TestRedactCommandLine_MasksKV(t *testing.T) {
	red := RedactCommandLine("export AWS_SECRET=value-here-xxx more args")
	if !contains_string(red, "AWS_SECRET=****") {
		t.Errorf("expected mask in: %q", red)
	}
	if contains_string(red, "value-here-xxx") {
		t.Errorf("plaintext leaked: %q", red)
	}
}

func TestRedactCommandLine_MasksBasicAuth(t *testing.T) {
	red := RedactCommandLine("curl -u admin:hunter2 https://api")
	if !contains_string(red, "admin:****") {
		t.Errorf("expected mask in: %q", red)
	}
	if contains_string(red, "hunter2") {
		t.Errorf("plaintext leaked: %q", red)
	}
}

// ---- Redact -------------------------------------------------------

func TestRedact_LongString(t *testing.T) {
	r := Redact("AKIAIOSFODNN7EXAMPLE_padding")
	if r[:4] != "AKIA" {
		t.Errorf("head = %q", r[:4])
	}
	if !contains_string(r, "****") {
		t.Errorf("missing **** in: %q", r)
	}
	if r[len(r)-4:] != "ding" {
		t.Errorf("tail = %q", r[len(r)-4:])
	}
}

func TestRedact_ShortStrings(t *testing.T) {
	if r := Redact("short"); r != "*****" {
		t.Errorf("got %q", r)
	}
	if r := Redact("12345678"); r != "********" {
		t.Errorf("got %q", r)
	}
	if r := Redact(""); r != "" {
		t.Errorf("got %q", r)
	}
}

// ---- IsOpensshEncrypted -------------------------------------------

func TestIsOpensshEncrypted_Marker(t *testing.T) {
	body := "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
		"<bytes>\nb3BlbnNzaC1rZXktdjEAAAAACmFlczI1Ni1jYmM=\n" +
		"-----END OPENSSH PRIVATE KEY-----"
	if !IsOpensshEncrypted(body) {
		t.Error("encrypted key should return true")
	}
}

func TestIsOpensshEncrypted_Unencrypted(t *testing.T) {
	body := "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
		"<bytes>\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmU\n" +
		"-----END OPENSSH PRIVATE KEY-----"
	if IsOpensshEncrypted(body) {
		t.Error("unencrypted key should return false")
	}
}

func TestIsOpensshEncrypted_NotOpenSSHKey(t *testing.T) {
	if IsOpensshEncrypted("-----BEGIN RSA PRIVATE KEY-----") {
		t.Error("non-openssh key should return false")
	}
}

// ---- IsBareAuthorizedKeyEntry -------------------------------------

func TestIsBareAuthorizedKey_RsaLine(t *testing.T) {
	if !IsBareAuthorizedKeyEntry("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ comment") {
		t.Error("expected bare ssh-rsa to match")
	}
}

func TestIsBareAuthorizedKey_OptionsLine(t *testing.T) {
	if IsBareAuthorizedKeyEntry(`from="10.0.0.0/8",no-pty ssh-rsa AAAAB...`) {
		t.Error("line with options should not match")
	}
}

func TestIsBareAuthorizedKey_Comment(t *testing.T) {
	if IsBareAuthorizedKeyEntry("# this is a comment") {
		t.Error("comment should not match")
	}
}

// ---- PgHbaTrustFinding --------------------------------------------

func TestPgHbaTrust_RemoteFlagged(t *testing.T) {
	got := PgHbaTrustFinding("host all all 10.0.0.0/8 trust")
	if got == "" {
		t.Error("remote trust should be flagged")
	}
}

func TestPgHbaTrust_LocalFine(t *testing.T) {
	if PgHbaTrustFinding("local all all trust") != "" {
		t.Error("local trust should not be flagged")
	}
	if PgHbaTrustFinding("host all all 127.0.0.1/32 trust") != "" {
		t.Error("loopback trust should not be flagged")
	}
	if PgHbaTrustFinding("host all all ::1/128 trust") != "" {
		t.Error("ipv6 loopback trust should not be flagged")
	}
}

func TestPgHbaTrust_NonTrust(t *testing.T) {
	if PgHbaTrustFinding("host all all 10.0.0.0/8 md5") != "" {
		t.Error("md5 should not be flagged")
	}
}

func TestPgHbaTrust_Comment(t *testing.T) {
	if PgHbaTrustFinding("# host all all 10.0.0.0/8 trust") != "" {
		t.Error("comment should not be flagged")
	}
}

// ---- RedisCheck ---------------------------------------------------

func TestRedis_NoAuthAndNoLoopback(t *testing.T) {
	body := "port 6379\n"
	noAuth, _ := RedisCheck(body)
	if !noAuth {
		t.Error("missing requirepass + missing bind should flag noAuth")
	}
}

func TestRedis_RequirePassFine(t *testing.T) {
	body := "requirepass strongsecret\n"
	noAuth, _ := RedisCheck(body)
	if noAuth {
		t.Error("requirepass set should not flag noAuth")
	}
}

func TestRedis_BindLoopbackFine(t *testing.T) {
	body := "bind 127.0.0.1 ::1\n"
	noAuth, _ := RedisCheck(body)
	if noAuth {
		t.Error("loopback bind should not flag noAuth")
	}
}

func TestRedis_ProtectedModeOff(t *testing.T) {
	body := "protected-mode no\nrequirepass x\n"
	_, protOff := RedisCheck(body)
	if !protOff {
		t.Error("protected-mode no should be flagged")
	}
}

// ---- MongoAuthEnabled ---------------------------------------------

func TestMongo_AuthEnabled(t *testing.T) {
	yaml := "net:\n  port: 27017\nsecurity:\n  authorization: enabled\n"
	if !MongoAuthEnabled(yaml) {
		t.Error("expected auth enabled")
	}
}

func TestMongo_AuthDisabled(t *testing.T) {
	yaml := "net:\n  port: 27017\nsecurity:\n  authorization: disabled\n"
	if MongoAuthEnabled(yaml) {
		t.Error("disabled should return false")
	}
}

func TestMongo_NoSecuritySection(t *testing.T) {
	if MongoAuthEnabled("net:\n  port: 27017\n") {
		t.Error("missing security: should return false")
	}
}

func TestMongo_AuthInOtherBlock(t *testing.T) {
	yaml := "operationProfiling:\n  authorization: enabled\nnet:\n  port: 27017\n"
	if MongoAuthEnabled(yaml) {
		t.Error("authorization under non-security should not count")
	}
}

// ---- IsWebappConfigFile -------------------------------------------

func TestWebappConfigFile_Known(t *testing.T) {
	for _, name := range []string{
		".env", ".env.production", "wp-config.php", "settings.py",
		"application.yaml", "appsettings.json", "appsettings.Development.json",
	} {
		if !IsWebappConfigFile(name) {
			t.Errorf("expected %q to match", name)
		}
	}
}

func TestWebappConfigFile_Skips(t *testing.T) {
	for _, name := range []string{"README.md", "index.php", "appsettings.txt"} {
		if IsWebappConfigFile(name) {
			t.Errorf("expected %q to NOT match", name)
		}
	}
}

// ---- SafeID + SplitParent -----------------------------------------

func TestSafeID(t *testing.T) {
	if SafeID("aws-access-token") != "aws-access-token" {
		t.Error("safe chars should pass through")
	}
	if SafeID("foo.bar") != "foo_bar" {
		t.Error("dots should become underscores")
	}
}

func TestSplitParent_Basic(t *testing.T) {
	parent, name, ok := SplitParent("/home/alice/.ssh/id_rsa")
	if !ok || parent != "/home/alice/.ssh" || name != "id_rsa" {
		t.Errorf("got (%q, %q, %v)", parent, name, ok)
	}
	parent, name, ok = SplitParent("/etc")
	if !ok || parent != "/" || name != "etc" {
		t.Errorf("got (%q, %q, %v)", parent, name, ok)
	}
}

func TestSplitParent_RootRejected(t *testing.T) {
	if _, _, ok := SplitParent("/"); ok {
		t.Error("'/' should return ok=false")
	}
	if _, _, ok := SplitParent(""); ok {
		t.Error("'' should return ok=false")
	}
}

// ---- helpers ------------------------------------------------------

func contains_string(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
