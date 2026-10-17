package store

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Transport identifiers.
const (
	MCPTransportStdio          = "stdio"
	MCPTransportStreamableHTTP = "streamable-http"
	MCPTransportSSE            = "sse"
)

// MCPClientConfig is one external MCP server connection, as persisted in
// mcp_clients.json.
type MCPClientConfig struct {
	// RPC edit token; stripped before persistence so existing file format stays compatible.
	Revision string `json:"revision,omitempty"`
	// Name is the user-facing label and the stable key used to namespace its
	// tools (mcp__<name>__<tool>). Required, unique, non-empty.
	Name string `json:"name"`
	// Transport is stdio | streamable-http | sse.
	Transport string `json:"transport"`
	// Command + Args run a stdio server subprocess. Used when Transport ==
	// stdio.
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	// Env are extra environment variables for the stdio subprocess.
	Env map[string]string `json:"env,omitempty"`
	// URL is the streamable-http / sse endpoint.
	URL string `json:"url,omitempty"`
	// Headers are static request headers (e.g. Authorization: Bearer ...) for
	// the HTTP transports.
	Headers map[string]string `json:"headers,omitempty"`
	// Enabled toggles the connection without deleting it.
	Enabled bool `json:"enabled"`
}

// Validate returns a readable error when the config cannot be connected.
func (c MCPClientConfig) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("mcp client name is required")
	}
	switch c.Transport {
	case MCPTransportStdio:
		if strings.TrimSpace(c.Command) == "" {
			return errors.New("stdio transport requires a command")
		}
	case MCPTransportStreamableHTTP, MCPTransportSSE:
		if strings.TrimSpace(c.URL) == "" {
			return fmt.Errorf("%s transport requires a url", c.Transport)
		}
	default:
		return fmt.Errorf("unknown transport %q (want %s|%s|%s)", c.Transport,
			MCPTransportStdio, MCPTransportStreamableHTTP, MCPTransportSSE)
	}
	return nil
}

// mcpClientsFile is the on-disk shape of mcp_clients.json.
type mcpClientsFile struct {
	Clients []MCPClientConfig `json:"clients"`
}

const mcpClientsFileName = "mcp_clients.json"

// MCPClientStore persists external tool servers with the shared store envelope.
// This is device-local configuration; it is not registered for cloud sync.
type MCPClientStore struct {
	configDir     string
	passwordStore PasswordStore
	mu            sync.Mutex
	lastSavedHash string
}

func NewMCPClientStore(configDir string) *MCPClientStore {
	return &MCPClientStore{configDir: configDir}
}

func (s *MCPClientStore) SetPasswordStore(ps PasswordStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.passwordStore = ps
}

func (s *MCPClientStore) filePath() string { return filepath.Join(s.configDir, mcpClientsFileName) }
func (s *MCPClientStore) Path() string     { return s.filePath() }

func (s *MCPClientStore) Save(clients []MCPClientConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(clients, false)
}

func (s *MCPClientStore) ForceSave(clients []MCPClientConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(clients, true)
}

func (s *MCPClientStore) saveLocked(clients []MCPClientConfig, force bool) error {
	seen := make(map[string]bool)
	clean := make([]MCPClientConfig, 0, len(clients))
	for _, cfg := range clients {
		cfg.Revision = ""
		cfg.Name = strings.TrimSpace(cfg.Name)
		if cfg.Name == "" || seen[cfg.Name] {
			continue
		}
		seen[cfg.Name] = true
		cfg.Args = append([]string(nil), cfg.Args...)
		cfg.Env = maps.Clone(cfg.Env)
		cfg.Headers = maps.Clone(cfg.Headers)
		clean = append(clean, cfg)
	}
	sort.Slice(clean, func(i, j int) bool {
		a, b := strings.ToLower(clean[i].Name), strings.ToLower(clean[j].Name)
		if a != b {
			return a < b
		}
		return clean[i].Name < clean[j].Name
	})
	if err := os.MkdirAll(s.configDir, 0700); err != nil {
		return err
	}
	skip := s.lastSavedHash
	if force {
		skip = forceWrite
	}
	data := mcpClientsFile{Clients: clean}
	h, err := saveEncryptedFile(mcpClientsFileName, s.filePath(), &data, s.passwordStore, skip)
	s.lastSavedHash = h
	return err
}

func (s *MCPClientStore) Load() ([]MCPClientConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, res, hash, existed, err := loadEncryptedFile[mcpClientsFile](mcpClientsFileName, s.filePath(), s.passwordStore)
	if errors.Is(err, errUnparseableConfig) {
		s.lastSavedHash = ""
		return []MCPClientConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !existed {
		s.lastSavedHash = ""
		return []MCPClientConfig{}, nil
	}
	if data.Clients == nil {
		data.Clients = []MCPClientConfig{}
	}
	if res.Plaintext > 0 {
		if err := s.saveLocked(data.Clients, true); err != nil {
			return nil, err
		}
	} else {
		s.lastSavedHash = hash
	}
	return data.Clients, nil
}
