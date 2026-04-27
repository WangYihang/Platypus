package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/WangYihang/Platypus/internal/utils/config"
)

// L2: the bundled config.docker.yml used to ship demo MinIO credentials
// (platypus_admin / platypus_password) inline. Anyone copy-pasting the
// file into production exposed their object store. Lock the example
// to blank credentials and require operators to inject real ones via
// env vars / a secret.
func TestConfigDockerYAMLHasNoDemoCredentials(t *testing.T) {
	repoRoot := repoRoot(t)
	path := filepath.Join(repoRoot, "config.docker.yml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(body)
	for _, banned := range []string{
		"platypus_admin",
		"platypus_password",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("config.docker.yml ships demo credential %q; replace with empty string + env-var injection", banned)
		}
	}
}

// L2: when the distributor is enabled (Endpoint set), the artifact
// store credentials MUST be non-empty. Letting the server boot with
// blank creds and a configured endpoint silently disables artefact
// uploads (or worse, lets unauthenticated reads through depending on
// the bucket policy). Validation must be loud, at startup.
func TestConfigValidate_DistributorRequiresStoreCredentials(t *testing.T) {
	cfg := &config.Config{
		Distributor: config.DistributorConfig{
			Store: config.ArtifactStoreConfig{
				Endpoint: "minio:9000",
				Bucket:   "platypus-artifacts",
				// AccessKeyID + SecretAccessKey deliberately empty.
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() with distributor endpoint set + blank credentials must return an error")
	}
	if !errors.Is(err, config.ErrMissingStoreCredentials) {
		t.Fatalf("Validate() returned %v; want ErrMissingStoreCredentials in chain", err)
	}
}

// L2: distributor disabled (Endpoint == "") → blank creds are fine,
// validation must pass.
func TestConfigValidate_DistributorDisabledIsOK(t *testing.T) {
	cfg := &config.Config{
		Distributor: config.DistributorConfig{
			Store: config.ArtifactStoreConfig{
				// Endpoint blank → distributor off, no creds needed.
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with distributor disabled returned %v; want nil", err)
	}
}

// L2: env-var fallback so docker-compose can inject MinIO creds via
// PLATYPUS_DISTRIBUTOR_STORE_ACCESS_KEY_ID / _SECRET_ACCESS_KEY into
// the running container without templating the YAML. Validation must
// see the env-var values, not just the YAML.
func TestConfigValidate_StoreCredentialsFromEnv(t *testing.T) {
	t.Setenv("PLATYPUS_DISTRIBUTOR_STORE_ACCESS_KEY_ID", "from-env")
	t.Setenv("PLATYPUS_DISTRIBUTOR_STORE_SECRET_ACCESS_KEY", "also-from-env")
	cfg := &config.Config{
		Distributor: config.DistributorConfig{
			Store: config.ArtifactStoreConfig{
				Endpoint: "minio:9000",
				Bucket:   "platypus-artifacts",
				// blank YAML — env should fill in.
			},
		},
	}
	if err := cfg.ApplyEnvOverrides(); err != nil {
		t.Fatalf("ApplyEnvOverrides: %v", err)
	}
	if cfg.Distributor.Store.AccessKeyID != "from-env" {
		t.Errorf("AccessKeyID = %q; want \"from-env\"", cfg.Distributor.Store.AccessKeyID)
	}
	if cfg.Distributor.Store.SecretAccessKey != "also-from-env" {
		t.Errorf("SecretAccessKey = %q; want \"also-from-env\"", cfg.Distributor.Store.SecretAccessKey)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() after env override: %v", err)
	}
}

// repoRoot walks up from the test's working dir until it finds go.mod.
// Lets the test resolve the project root so config.docker.yml is found
// regardless of `go test` invocation directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod walking up from %s", dir)
		}
		dir = parent
	}
}

// Sanity that the test file even compiles against viper — config.Config
// uses viper-style mapstructure tags, and we want a one-line check that
// the env binding mirror works without a YAML file present.
func TestViperUnmarshalShape(t *testing.T) {
	v := viper.New()
	v.Set("distributor.store.endpoint", "x")
	var c config.Config
	if err := v.Unmarshal(&c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if c.Distributor.Store.Endpoint != "x" {
		t.Fatalf("expected endpoint=x, got %q", c.Distributor.Store.Endpoint)
	}
}
