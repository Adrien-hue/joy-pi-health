package ci_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	checkoutPin = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
	setupGoPin  = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
	uploadPin   = "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a"
	trixieImage = "debian:trixie-slim@sha256:d7e12182ce18b85b93007c1dedf31f2d29e01ccf3182cc4017c709b6259bc132"
)

func TestWorkflowSecurityAndActionPins(t *testing.T) {
	for _, name := range []string{"quality.yml", "release-validation.yml"} {
		workflow := read(t, ".github", "workflows", name)
		if !regexp.MustCompile(`(?m)^permissions:\s*\n\s+contents: read\s*$`).MatchString(workflow) {
			t.Errorf("%s does not declare read-only contents permission", name)
		}
		for _, forbidden := range []string{"contents: write", "pull_request_target:", "id-token: write", "packages: write", "deployments: write", "gh release", "git push"} {
			if strings.Contains(workflow, forbidden) {
				t.Errorf("%s contains forbidden publishing capability %q", name, forbidden)
			}
		}
		for _, match := range regexp.MustCompile(`uses:\s*([^\s#]+)`).FindAllStringSubmatch(workflow, -1) {
			if !regexp.MustCompile(`^actions/(checkout|setup-go|upload-artifact)@[0-9a-f]{40}$`).MatchString(match[1]) {
				t.Errorf("%s uses unapproved or mutable action %q", name, match[1])
			}
		}
	}
}

func TestQualityWorkflowContract(t *testing.T) {
	workflow := read(t, ".github", "workflows", "quality.yml")
	for _, required := range []string{
		checkoutPin, setupGoPin, "go-version: 1.27.1", "cache: false", "ubuntu-24.04", "windows-2025",
		"pull_request:", "develop", "main", "workflow_dispatch:", "go mod tidy -diff", "gofmt -l cmd internal",
		"go test ./...", "go vet ./...", "go build ./...", "go run ./cmd/joy-pi-health --help",
		"CGO_ENABLED", "GOOS: linux", "GOARCH: arm64", "GOARM64: v8.0", "./test/packaging", "./test/ci",
		"bash -n test/release/validate-artifacts.sh test/release/pi3bplus/*.sh",
		"sh test/release/pi3bplus/model-validation-test.sh",
		"sh test/release/pi3bplus/cpu-methodology-test.sh",
		"sh test/release/pi3bplus/package-baseline-test.sh",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("quality workflow is missing %q", required)
		}
	}
	if !regexp.MustCompile(`(?m)^\s+- name: Verify Raspberry Pi 3B\+ release harness\s*\n\s+if: runner\.os == 'Linux'\s*$`).MatchString(workflow) {
		t.Error("quality workflow must restrict the Raspberry Pi release harness step to Linux")
	}
}

func TestReleaseWorkflowContract(t *testing.T) {
	workflow := read(t, ".github", "workflows", "release-validation.yml")
	for _, required := range []string{
		checkoutPin, setupGoPin, uploadPin, trixieImage, "go-version: 1.27.1", "cache: false",
		"sh debian/build-release.sh", "sh test/release/validate-artifacts.sh dist", "SOURCE_DATE_EPOCH",
		"dpkg-dev", "lintian", "systemd", "retention-days: 14", "joy-pi-health-ci-validation-",
		"joy-pi-health_0.1.0-1_arm64.deb", "joy-pi-health-v0.1.0-linux-arm64.tar.gz",
		"joy-pi-health-v0.1.0-release.json", "SHA256SUMS",
		"dash -n debian/build-release.sh debian/postinst debian/prerm debian/postrm test/release/validate-artifacts.sh test/release/pi3bplus/*.sh",
		"sh test/release/pi3bplus/model-validation-test.sh",
		"sh test/release/pi3bplus/cpu-methodology-test.sh",
		"sh test/release/pi3bplus/package-baseline-test.sh",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing %q", required)
		}
	}
	if strings.Contains(workflow, "go build ") {
		t.Error("release workflow duplicates release construction instead of using the canonical builder")
	}
	if strings.Count(workflow, "sh debian/build-release.sh") != 2 {
		t.Error("release workflow must perform exactly two independent canonical builds")
	}
	uploadAt := strings.Index(workflow, uploadPin)
	lastValidationAt := strings.LastIndex(workflow, "sh test/release/validate-artifacts.sh dist")
	if uploadAt < lastValidationAt {
		t.Error("release artifacts are uploaded before final validation")
	}
}

func TestReleaseValidatorCoversArtifactContract(t *testing.T) {
	validator := read(t, "test", "release", "validate-artifacts.sh")
	for _, required := range []string{
		"sha256sum --check", "dpkg-deb --field", "dpkg-deb --contents", "systemd-analyze verify",
		"lintian --fail-on error", "readelf -l", "SupplementaryGroups=video", "DevicePolicy=closed",
		"deb-systemd-invoke", "third_party_dependencies", "firmware_access", "git rev-parse HEAD",
	} {
		if !strings.Contains(validator, required) {
			t.Errorf("release validator is missing %q", required)
		}
	}
}

func read(t *testing.T, elements ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, elements...)...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
