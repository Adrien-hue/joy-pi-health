package packaging_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPackageMetadata(t *testing.T) {
	control := read(t, "debian", "control.in")
	for _, required := range []string{
		"Package: joy-pi-health",
		"Version: @VERSION@",
		"Architecture: arm64",
		"init-system-helpers (>= 1.60)",
		"systemd (>= 254)",
	} {
		if !strings.Contains(control, required) {
			t.Errorf("control.in is missing %q", required)
		}
	}
	changelog := read(t, "debian", "changelog")
	if !strings.HasPrefix(changelog, "joy-pi-health (0.1.0-1) unstable; urgency=medium") {
		t.Error("Debian changelog does not identify the canonical package version")
	}
	overrides := read(t, "debian", "lintian-overrides")
	for _, intentional := range []string{"statically-linked-binary", "unstripped-binary-or-object"} {
		if !strings.Contains(overrides, intentional) {
			t.Errorf("lintian overrides do not document %q", intentional)
		}
	}
}

func TestSystemdContract(t *testing.T) {
	unit := read(t, "debian", "joy-pi-health.service.in")
	required := []string{
		"Type=notify", "NotifyAccess=main", "ExecStart=/usr/bin/joy-pi-health",
		"User=_joy-pi-health", "Group=_joy-pi-health", "Restart=on-failure",
		"RestartPreventExitStatus=2", "RestartSec=1s", "RestartSteps=5",
		"RestartMaxDelaySec=30s", "TimeoutStopSec=2s", "NoNewPrivileges=yes",
		"ProtectSystem=strict", "ProtectHome=yes", "MemoryDenyWriteExecute=yes",
		"CapabilityBoundingSet=", "AmbientCapabilities=", "DevicePolicy=closed",
		"DeviceAllow=/dev/vcio r", "DeviceAllow=/dev/vcio_gencmd r", "UMask=0077",
	}
	for _, value := range required {
		if !strings.Contains(unit, value) {
			t.Errorf("service unit is missing %q", value)
		}
	}
	for _, forbidden := range []string{"User=root", "PrivateDevices=yes", "ReadWritePaths=", "Environment=", "EnvironmentFile="} {
		if strings.Contains(unit, forbidden) {
			t.Errorf("service unit contains forbidden directive %q", forbidden)
		}
	}
}

func TestServiceIdentity(t *testing.T) {
	sysusers := strings.TrimSpace(read(t, "debian", "joy-pi-health.sysusers"))
	if sysusers != `u _joy-pi-health - "Joy Pi Health service" /nonexistent /usr/sbin/nologin` {
		t.Fatalf("unexpected sysusers declaration: %q", sysusers)
	}
	for _, script := range []string{"postinst", "prerm", "postrm"} {
		if strings.Contains(read(t, "debian", script), "userdel") || strings.Contains(read(t, "debian", script), "groupdel") {
			t.Errorf("%s removes the retained service identity", script)
		}
	}
}

func TestMaintainerScriptsUsePolicyAwareHelpers(t *testing.T) {
	postinst := read(t, "debian", "postinst")
	prerm := read(t, "debian", "prerm")
	for _, required := range []string{
		"deb-systemd-helper enable", "deb-systemd-invoke start", "deb-systemd-invoke try-restart",
	} {
		if !strings.Contains(postinst, required) {
			t.Errorf("postinst is missing %q", required)
		}
	}
	for _, required := range []string{"deb-systemd-invoke stop", "deb-systemd-helper disable"} {
		if !strings.Contains(prerm, required) {
			t.Errorf("prerm is missing %q", required)
		}
	}
	directControl := regexp.MustCompile(`(?m)systemctl(?:\s+--system)?\s+(?:start|stop|restart|try-restart|enable|disable)\b`)
	for _, script := range []string{"postinst", "prerm", "postrm"} {
		if directControl.MatchString(read(t, "debian", script)) {
			t.Errorf("%s contains direct systemctl service control", script)
		}
	}
	if !strings.Contains(postinst, `if [ -z "${2:-}" ]`) || !strings.Contains(postinst, "else\n        deb-systemd-invoke try-restart") {
		t.Error("postinst does not distinguish fresh installation from upgrade/reinstall/downgrade")
	}
	if !strings.Contains(prerm, `if [ "${1:-}" = remove ]`) {
		t.Error("prerm service removal is not limited to the remove action")
	}
	for _, script := range []string{"postinst", "prerm", "postrm"} {
		contents := read(t, "debian", script)
		if strings.Contains(contents, "/etc/systemd/system/joy-pi-health.service.d") || strings.Contains(contents, "journalctl --vacuum") {
			t.Errorf("%s alters administrator-owned state", script)
		}
	}
}

func TestPermissionProfilesAreNarrow(t *testing.T) {
	unit := read(t, "debian", "joy-pi-health.service.in")
	if strings.Count(unit, "DeviceAllow=") != 2 || strings.Contains(unit, "DeviceAllow=/dev/*") {
		t.Fatal("service device policy is not limited to the two firmware devices")
	}
	rule := read(t, "debian", "firmware-acl", "70-joy-pi-health-vcio-acl.rules")
	for _, required := range []string{`KERNEL=="vcio"`, `KERNEL=="vcio_gencmd"`, "u:_joy-pi-health:r--"} {
		if !strings.Contains(rule, required) {
			t.Errorf("ACL fallback is missing %q", required)
		}
	}
	for _, forbidden := range []string{"chmod", "chown", "MODE=", "GROUP=\"video\""} {
		if strings.Contains(rule, forbidden) {
			t.Errorf("ACL fallback contains broad permission operation %q", forbidden)
		}
	}
}

func TestReleaseBuilderContract(t *testing.T) {
	script := read(t, "debian", "build-release.sh")
	for _, required := range []string{
		"version=0.1.0-1", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64", "GOARM64=v8.0",
		"go1.27.1", "-trimpath", "-buildvcs=false", "cmp -s", "--root-owner-group",
		"--uniform-compression", "--compression=gzip", "--compression-level=9",
		"--sort=name", "gzip -n", "sha256sum", "third_party_dependencies", "@FIRMWARE_ACCESS@",
		"install -m 0755", "install -m 0644", "dpkg-deb --extract",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("release builder is missing %q", required)
		}
	}
	if strings.Contains(script, "AGENTS.md") {
		t.Fatal("release builder includes repository-only AGENTS.md")
	}
	for _, forbidden := range []string{"internal/", "cmd/", "test/"} {
		if strings.Contains(script, `install -m 0644 `+forbidden) {
			t.Errorf("release builder installs source-tree content from %q", forbidden)
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
