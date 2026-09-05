package config

import (
	"errors"
	"net/netip"
	"testing"
)

func TestResolveDefaults(t *testing.T) {
	t.Parallel()

	got, err := Resolve(nil, nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertConfig(t, got, "127.0.0.1", 8080, false, LogLevelInfo)
}

func TestResolvePrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		environment []string
		wantAddress string
		wantPort    uint16
		wantAllow   bool
		wantLevel   LogLevel
	}{
		{
			name:        "environment overrides defaults",
			environment: []string{"JOY_PI_HEALTH_LISTEN_ADDRESS=127.0.0.2", "JOY_PI_HEALTH_LISTEN_PORT=9090", "JOY_PI_HEALTH_ALLOW_NON_LOOPBACK=true", "JOY_PI_HEALTH_LOG_LEVEL=warn"},
			wantAddress: "127.0.0.2",
			wantPort:    9090,
			wantAllow:   true,
			wantLevel:   LogLevelWarn,
		},
		{
			name:        "command line overrides environment",
			args:        []string{"--listen-address", "127.0.0.3", "--listen-port=9091", "--allow-non-loopback=false", "--log-level", "error"},
			environment: []string{"JOY_PI_HEALTH_LISTEN_ADDRESS=127.0.0.2", "JOY_PI_HEALTH_LISTEN_PORT=9090", "JOY_PI_HEALTH_ALLOW_NON_LOOPBACK=true", "JOY_PI_HEALTH_LOG_LEVEL=warn"},
			wantAddress: "127.0.0.3",
			wantPort:    9091,
			wantAllow:   false,
			wantLevel:   LogLevelError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := Resolve(test.args, test.environment)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			assertConfig(t, got, test.wantAddress, test.wantPort, test.wantAllow, test.wantLevel)
		})
	}
}

func TestResolveIgnoresOverriddenRecognizedEnvironmentValues(t *testing.T) {
	t.Parallel()

	got, err := Resolve(
		[]string{"--listen-address=127.0.0.4", "--listen-port=8081", "--allow-non-loopback=false", "--log-level=debug"},
		[]string{"JOY_PI_HEALTH_LISTEN_ADDRESS=not-an-address", "JOY_PI_HEALTH_LISTEN_PORT=invalid", "JOY_PI_HEALTH_ALLOW_NON_LOOPBACK=invalid", "JOY_PI_HEALTH_LOG_LEVEL=invalid"},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertConfig(t, got, "127.0.0.4", 8081, false, LogLevelDebug)
}

func TestResolveUsesLastEffectiveEnvironmentValue(t *testing.T) {
	t.Parallel()

	got, err := Resolve(nil, []string{
		"JOY_PI_HEALTH_LISTEN_PORT=8081",
		"JOY_PI_HEALTH_LISTEN_PORT=8082",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.ListenPort() != 8082 {
		t.Errorf("ListenPort() = %d, want 8082", got.ListenPort())
	}
}

func TestResolveEnvironmentNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		environment []string
		wantError   bool
	}{
		{name: "unrelated variable ignored", environment: []string{"PATH=ignored"}},
		{name: "lowercase prefix ignored", environment: []string{"joy_pi_health_UNKNOWN=ignored"}},
		{name: "unknown reserved key", environment: []string{"JOY_PI_HEALTH_UNKNOWN=value"}, wantError: true},
		{name: "empty reserved suffix", environment: []string{"JOY_PI_HEALTH_=value"}, wantError: true},
		{name: "unknown reserved key despite CLI", environment: []string{"JOY_PI_HEALTH_UNKNOWN=value"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			args := []string(nil)
			if test.name == "unknown reserved key despite CLI" {
				args = []string{"--listen-port=8081"}
			}
			_, err := Resolve(args, test.environment)
			assertErrorPresence(t, err, test.wantError)
		})
	}
}

func TestResolveCommandLineGrammar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown option", args: []string{"--unknown=value"}},
		{name: "single dash", args: []string{"-h"}},
		{name: "positional", args: []string{"value"}},
		{name: "terminator", args: []string{"--"}},
		{name: "help with value", args: []string{"--help=true"}},
		{name: "help with another argument", args: []string{"--help", "--listen-port=8081"}},
		{name: "missing value", args: []string{"--listen-port"}},
		{name: "missing value before option", args: []string{"--listen-port", "--log-level=info"}},
		{name: "empty equals value", args: []string{"--listen-port="}},
		{name: "bare boolean", args: []string{"--allow-non-loopback"}},
		{name: "repeated equals option", args: []string{"--listen-port=8081", "--listen-port=8082"}},
		{name: "repeated mixed syntax", args: []string{"--listen-port", "8081", "--listen-port=8082"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve(test.args, nil)
			assertConfigurationError(t, err)
		})
	}
}

func TestResolveEmptySelectedValues(t *testing.T) {
	t.Parallel()

	for _, argument := range []string{
		"--listen-address=",
		"--listen-port=",
		"--allow-non-loopback=",
		"--log-level=",
	} {
		t.Run("CLI "+argument, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve([]string{argument}, nil)
			assertConfigurationError(t, err)
		})
	}

	for _, entry := range []string{
		"JOY_PI_HEALTH_LISTEN_ADDRESS=",
		"JOY_PI_HEALTH_LISTEN_PORT=",
		"JOY_PI_HEALTH_ALLOW_NON_LOOPBACK=",
		"JOY_PI_HEALTH_LOG_LEVEL=",
	} {
		t.Run("environment "+entry, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve(nil, []string{entry})
			assertConfigurationError(t, err)
		})
	}
}

func TestResolveAddresses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		address   string
		allow     bool
		wantError bool
	}{
		{name: "loopback", address: "127.0.0.1"},
		{name: "loopback range boundary", address: "127.255.255.255"},
		{name: "wildcard acknowledged", address: "0.0.0.0", allow: true},
		{name: "private unicast acknowledged", address: "192.168.1.2", allow: true},
		{name: "public unicast acknowledged", address: "8.8.8.8", allow: true},
		{name: "link local acknowledged", address: "169.254.1.2", allow: true},
		{name: "wildcard without acknowledgement", address: "0.0.0.0", wantError: true},
		{name: "non-loopback without acknowledgement", address: "192.168.1.2", wantError: true},
		{name: "multicast lower boundary", address: "224.0.0.0", allow: true, wantError: true},
		{name: "multicast upper boundary", address: "239.255.255.255", allow: true, wantError: true},
		{name: "limited broadcast", address: "255.255.255.255", allow: true, wantError: true},
		{name: "IPv6", address: "::1", allow: true, wantError: true},
		{name: "IPv4 mapped IPv6", address: "::ffff:127.0.0.1", allow: true, wantError: true},
		{name: "hostname", address: "localhost", wantError: true},
		{name: "CIDR", address: "127.0.0.1/8", wantError: true},
		{name: "leading zero", address: "127.0.0.01", wantError: true},
		{name: "surrounding whitespace", address: " 127.0.0.1", wantError: true},
		{name: "malformed", address: "127.0.0", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			allow := "false"
			if test.allow {
				allow = "true"
			}
			got, err := Resolve([]string{"--listen-address=" + test.address, "--allow-non-loopback=" + allow}, nil)
			assertErrorPresence(t, err, test.wantError)
			if err == nil && got.ListenAddress() != netip.MustParseAddr(test.address) {
				t.Errorf("ListenAddress() = %v, want %s", got.ListenAddress(), test.address)
			}
		})
	}
}

func TestResolvePorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value     string
		want      uint16
		wantError bool
	}{
		{value: "1", want: 1},
		{value: "65535", want: 65535},
		{value: "08080", want: 8080},
		{value: "0", wantError: true},
		{value: "65536", wantError: true},
		{value: "-1", wantError: true},
		{value: "+1", wantError: true},
		{value: " 1", wantError: true},
		{value: "1.5", wantError: true},
		{value: "port", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()
			got, err := Resolve([]string{"--listen-port=" + test.value}, nil)
			assertErrorPresence(t, err, test.wantError)
			if err == nil && got.ListenPort() != test.want {
				t.Errorf("ListenPort() = %d, want %d", got.ListenPort(), test.want)
			}
		})
	}
}

func TestResolveBooleans(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"true", "false"} {
		t.Run("accepted "+value, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve([]string{"--allow-non-loopback=" + value}, nil)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
		})
	}
	for _, value := range []string{"True", "FALSE", "1", "0", "yes", " true"} {
		t.Run("rejected "+value, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve([]string{"--allow-non-loopback=" + value}, nil)
			assertConfigurationError(t, err)
		})
	}
}

func TestResolveLogLevels(t *testing.T) {
	t.Parallel()

	for _, level := range []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError} {
		t.Run("accepted "+string(level), func(t *testing.T) {
			t.Parallel()
			got, err := Resolve([]string{"--log-level=" + string(level)}, nil)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.LogLevel() != level {
				t.Errorf("LogLevel() = %q, want %q", got.LogLevel(), level)
			}
		})
	}
	for _, level := range []string{"DEBUG", "Info", "warning", "trace", " info"} {
		t.Run("rejected "+level, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve([]string{"--log-level=" + level}, nil)
			assertConfigurationError(t, err)
		})
	}
}

func TestResolveHelp(t *testing.T) {
	t.Parallel()

	_, err := Resolve([]string{"--help"}, []string{"JOY_PI_HEALTH_UNKNOWN=value"})
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("Resolve() error = %v, want ErrHelp", err)
	}
	if HelpText == "" || HelpText[len(HelpText)-1] != '\n' {
		t.Error("HelpText must be non-empty and newline-terminated")
	}
}

func assertConfig(t *testing.T, got Config, address string, port uint16, allow bool, level LogLevel) {
	t.Helper()
	if got.ListenAddress() != netip.MustParseAddr(address) {
		t.Errorf("ListenAddress() = %v, want %s", got.ListenAddress(), address)
	}
	if got.ListenPort() != port {
		t.Errorf("ListenPort() = %d, want %d", got.ListenPort(), port)
	}
	if got.AllowNonLoopback() != allow {
		t.Errorf("AllowNonLoopback() = %t, want %t", got.AllowNonLoopback(), allow)
	}
	if got.LogLevel() != level {
		t.Errorf("LogLevel() = %q, want %q", got.LogLevel(), level)
	}
}

func assertConfigurationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Resolve() error = nil, want configuration error")
	}
	if errors.Is(err, ErrHelp) {
		t.Fatalf("Resolve() error = ErrHelp, want configuration error")
	}
	var configurationErr *Error
	if !errors.As(err, &configurationErr) {
		t.Fatalf("Resolve() error type = %T, want *Error", err)
	}
}

func assertErrorPresence(t *testing.T, err error, want bool) {
	t.Helper()
	if want {
		assertConfigurationError(t, err)
		return
	}
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
}
