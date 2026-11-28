package config

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"
)

// readEnvFile parses NAME=value lines as systemd's EnvironmentFile does for
// simple values: blank lines and lines starting with # or ; are ignored and
// one pair of surrounding quotes is removed. There are no escapes.
func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return nil, fmt.Errorf("%s is UTF-16; save it as UTF-8", path)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // Notepad's byte order mark
	values := map[string]string{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			// The line is not echoed: it may hold a secret.
			return nil, fmt.Errorf("%s line %d: expected NAME=value", path, i+1)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		values[name] = value
	}
	return values, nil
}

// lookup resolves ${NAME}: process environment, then the env file, then, for
// commands that sign in nowhere, a placeholder.
func (c *Config) lookup(env func(string) (string, bool), file map[string]string, placeholder bool) func(string) (string, bool) {
	return func(name string) (string, bool) {
		if v, ok := env(name); ok {
			if _, inFile := file[name]; !inFile && !slices.Contains(c.FromProcessEnv, name) {
				c.FromProcessEnv = append(c.FromProcessEnv, name)
			}
			return v, true
		}
		if v, ok := file[name]; ok {
			return v, true
		}
		if placeholder {
			return "unset", true
		}
		return "", false
	}
}
