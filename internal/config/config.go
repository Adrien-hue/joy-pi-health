// Package config resolves Joy Pi Health's immutable startup configuration.
package config

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
)

const (
	optionListenAddress    = "listen-address"
	optionListenPort       = "listen-port"
	optionAllowNonLoopback = "allow-non-loopback"
	optionLogLevel         = "log-level"
	optionHelp             = "help"

	environmentPrefix           = "JOY_PI_HEALTH_"
	environmentListenAddress    = environmentPrefix + "LISTEN_ADDRESS"
	environmentListenPort       = environmentPrefix + "LISTEN_PORT"
	environmentAllowNonLoopback = environmentPrefix + "ALLOW_NON_LOOPBACK"
	environmentLogLevel         = environmentPrefix + "LOG_LEVEL"

	defaultListenAddress    = "127.0.0.1"
	defaultListenPort       = "8080"
	defaultAllowNonLoopback = "false"
	defaultLogLevel         = "info"
)

// HelpText is the command-line help displayed by the application.
const HelpText = `Usage: joy-pi-health [options]

Options:
  --listen-address <IPv4>       Listener IPv4 address (default: 127.0.0.1)
  --listen-port <port>          Listener TCP port (default: 8080)
  --allow-non-loopback <bool>   Acknowledge wildcard or non-loopback exposure (default: false)
  --log-level <level>           Log level: debug, info, warn, or error (default: info)
  --help                        Show this help and exit

Values may also use --name=value syntax. Command-line values override
JOY_PI_HEALTH_* environment values, which override built-in defaults.
`

// ErrHelp indicates that the sole command-line argument requested help.
var ErrHelp = errors.New("help requested")

// LogLevel is a validated logging severity.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Config is a fully resolved, immutable startup configuration.
type Config struct {
	listenAddress    netip.Addr
	listenPort       uint16
	allowNonLoopback bool
	logLevel         LogLevel
}

// ListenAddress returns the configured listener address.
func (c Config) ListenAddress() netip.Addr { return c.listenAddress }

// ListenPort returns the configured listener port.
func (c Config) ListenPort() uint16 { return c.listenPort }

// AllowNonLoopback reports whether wildcard or non-loopback binding was acknowledged.
func (c Config) AllowNonLoopback() bool { return c.allowNonLoopback }

// LogLevel returns the configured logging level.
func (c Config) LogLevel() LogLevel { return c.logLevel }

// Error is a safe, bounded configuration diagnostic.
type Error struct {
	message string
}

func (e *Error) Error() string { return e.message }

// Resolve applies command-line, environment, and default precedence and then
// validates the complete effective configuration.
func Resolve(args, environment []string) (Config, error) {
	if len(args) == 1 && args[0] == "--"+optionHelp {
		return Config{}, ErrHelp
	}

	commandLine, err := parseCommandLine(args)
	if err != nil {
		return Config{}, err
	}

	effectiveEnvironment, err := parseEnvironment(environment)
	if err != nil {
		return Config{}, err
	}

	addressText := selectValue(commandLine, optionListenAddress, effectiveEnvironment, environmentListenAddress, defaultListenAddress)
	portText := selectValue(commandLine, optionListenPort, effectiveEnvironment, environmentListenPort, defaultListenPort)
	allowText := selectValue(commandLine, optionAllowNonLoopback, effectiveEnvironment, environmentAllowNonLoopback, defaultAllowNonLoopback)
	logLevelText := selectValue(commandLine, optionLogLevel, effectiveEnvironment, environmentLogLevel, defaultLogLevel)

	address, err := parseAddress(addressText)
	if err != nil {
		return Config{}, err
	}
	port, err := parsePort(portText)
	if err != nil {
		return Config{}, err
	}
	allowNonLoopback, err := parseBoolean(allowText)
	if err != nil {
		return Config{}, err
	}
	logLevel, err := parseLogLevel(logLevelText)
	if err != nil {
		return Config{}, err
	}

	if !address.IsLoopback() && !allowNonLoopback {
		return Config{}, configurationError("wildcard or non-loopback binding requires allow-non-loopback=true")
	}

	return Config{
		listenAddress:    address,
		listenPort:       port,
		allowNonLoopback: allowNonLoopback,
		logLevel:         logLevel,
	}, nil
}

func parseCommandLine(args []string) (map[string]string, error) {
	values := make(map[string]string, 4)

	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "--") {
			return nil, configurationError("positional and single-dash arguments are not supported")
		}

		nameAndValue := strings.TrimPrefix(argument, "--")
		name, value, hasEquals := strings.Cut(nameAndValue, "=")
		if name == optionHelp {
			return nil, configurationError("--help must be used as the sole argument")
		}
		if !isKnownOption(name) {
			return nil, configurationError("unknown command-line option")
		}
		if _, exists := values[name]; exists {
			return nil, configurationError("command-line option --" + name + " was repeated")
		}

		if !hasEquals {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return nil, configurationError("command-line option --" + name + " requires a value")
			}
			index++
			value = args[index]
		}
		if value == "" {
			return nil, configurationError("command-line option --" + name + " cannot be empty")
		}

		values[name] = value
	}

	return values, nil
}

func parseEnvironment(entries []string) (map[string]string, error) {
	values := make(map[string]string, 4)

	for _, entry := range entries {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			value = ""
		}
		if strings.HasPrefix(name, environmentPrefix) && !isKnownEnvironment(name) {
			return nil, configurationError("unknown JOY_PI_HEALTH_ environment variable")
		}
		if isKnownEnvironment(name) {
			values[name] = value
		}
	}

	return values, nil
}

func selectValue(commandLine map[string]string, option string, environment map[string]string, key, fallback string) string {
	if value, exists := commandLine[option]; exists {
		return value
	}
	if value, exists := environment[key]; exists {
		return value
	}
	return fallback
}

func parseAddress(value string) (netip.Addr, error) {
	address, err := netip.ParseAddr(value)
	if err != nil || !address.Is4() || address.String() != value {
		return netip.Addr{}, configurationError("listen address must be a canonical IPv4 literal")
	}
	if address.IsMulticast() {
		return netip.Addr{}, configurationError("multicast listen addresses are not supported")
	}
	if address == netip.MustParseAddr("255.255.255.255") {
		return netip.Addr{}, configurationError("broadcast listen addresses are not supported")
	}
	return address, nil
}

func parsePort(value string) (uint16, error) {
	if value == "" {
		return 0, configurationError("listen port cannot be empty")
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, configurationError("listen port must be a decimal integer from 1 through 65535")
		}
	}

	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil || port == 0 {
		return 0, configurationError("listen port must be a decimal integer from 1 through 65535")
	}
	return uint16(port), nil
}

func parseBoolean(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, configurationError("allow-non-loopback must be exactly true or false")
	}
}

func parseLogLevel(value string) (LogLevel, error) {
	switch LogLevel(value) {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return LogLevel(value), nil
	default:
		return "", configurationError("log level must be debug, info, warn, or error")
	}
}

func isKnownOption(name string) bool {
	switch name {
	case optionListenAddress, optionListenPort, optionAllowNonLoopback, optionLogLevel:
		return true
	default:
		return false
	}
}

func isKnownEnvironment(name string) bool {
	switch name {
	case environmentListenAddress, environmentListenPort, environmentAllowNonLoopback, environmentLogLevel:
		return true
	default:
		return false
	}
}

func configurationError(message string) *Error {
	return &Error{message: message}
}
