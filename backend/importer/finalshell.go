package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ys-ll/uniterm/backend/session"
)

// FormatFinalShell imports connections from a FinalShell conn directory (one
// JSON pair per entry: folder.json defines groups, *_connect_config.json holds
// connections). Passwords are FinalShell-local encrypted and cannot be
// recovered across clients, so they are never imported — the connection is
// created expecting the password to be filled on first connect.
const FormatFinalShell = "finalshell"

// FinalShell's own JSON spells the field "conection_type" (one "n"); keep it.
// Observed values: 1 and 100 are SSH (1 appears in older 3.x exports, 100 in
// current builds), 101 is RDP.
const (
	finalShellTypeSSH = 100
	finalShellTypeRDP = 101
)

type finalShellFolder struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ParentID   string `json:"parent_id"`
	DeleteTime uint64 `json:"delete_time"`
}

type finalShellConnection struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	UserName       string `json:"user_name"`
	ParentID       string `json:"parent_id"`
	ConnectionType int    `json:"conection_type"`
	Description    string `json:"description"`
	DeleteTime     uint64 `json:"delete_time"`
	// RDP-only geometry; width/height only apply when custom_size is true.
	CustomSize bool `json:"custom_size"`
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Fullscreen bool `json:"fullscreen"`
}

// finalShellState accumulates the scanned entries until conversion.
type finalShellState struct {
	folders map[string]finalShellFolder
	orders  []finalShellConnection
	warns   []string
}

func parseFinalShell(srcPath string) (*ImportResult, error) {
	if srcPath == "" {
		var ok bool
		srcPath, ok = defaultFinalShellPath()
		if !ok {
			return nil, fmt.Errorf("cannot auto-detect the FinalShell conn directory; pick it manually")
		}
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("FinalShell import source must be the conn directory")
	}
	st := &finalShellState{folders: map[string]finalShellFolder{}}
	if err := scanFinalShellDir(srcPath, st); err != nil {
		return nil, err
	}
	return finalShellResult(st), nil
}

// defaultFinalShellPath probes the known FinalShell data locations. 3.x and
// portable builds use ~/.finalshell/conn; current Windows installs keep the
// profile under %LOCALAPPDATA%/finalshell.
func defaultFinalShellPath() (string, bool) {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if p := filepath.Join(home, ".finalshell", "conn"); finalShellDirHasConnections(p) {
			return p, true
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		if p := filepath.Join(local, "finalshell", "conn"); finalShellDirHasConnections(p) {
			return p, true
		}
	}
	return "", false
}

func finalShellDirHasConnections(dir string) bool {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name()), "_connect_config.json") {
			return true
		}
	}
	return false
}

func scanFinalShellDir(root string, st *finalShellState) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		st.absorb(d.Name(), raw)
		return nil
	})
}

// absorb classifies one scanned file by its base name.
func (st *finalShellState) absorb(name string, raw []byte) {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	switch {
	case base == "folder.json":
		var folder finalShellFolder
		if json.Unmarshal(raw, &folder) != nil {
			return
		}
		// delete_time != 0 marks an entry sitting in FinalShell's trash.
		if folder.DeleteTime == 0 && folder.ID != "" {
			st.folders[folder.ID] = folder
		}
	case strings.HasSuffix(strings.ToLower(base), "_connect_config.json"):
		var conn finalShellConnection
		if json.Unmarshal(raw, &conn) != nil {
			return
		}
		st.orders = append(st.orders, conn)
	}
}

// finalShellResult converts the scanned entries into the normalized result.
// SSH types accept both 1 (older builds) and 100 (current); 101 imports as an
// RDP connection; anything else is skipped and reported.
func finalShellResult(st *finalShellState) *ImportResult {
	res := &ImportResult{}
	pathMap := map[string]string{}
	for _, conn := range st.orders {
		if conn.DeleteTime != 0 {
			continue
		}
		var typ string
		switch conn.ConnectionType {
		case 1, finalShellTypeSSH:
			typ = "ssh"
		case finalShellTypeRDP:
			typ = "rdp"
		default:
			st.warns = append(st.warns, fmt.Sprintf("skipped unsupported FinalShell connection type %d: %s",
				conn.ConnectionType, displayNameOr(conn.Name, conn.Host)))
			continue
		}
		host := strings.TrimSpace(conn.Host)
		if host == "" {
			st.warns = append(st.warns, "skipped FinalShell connection with empty host: "+
				displayNameOr(conn.Name, conn.Host))
			continue
		}
		port := conn.Port
		if port <= 0 {
			port = defaultPort(typ)
		}
		user := strings.TrimSpace(conn.UserName)
		if user == "" && typ == "ssh" {
			user = "root"
		}
		name := strings.TrimSpace(conn.Name)
		if name == "" {
			name = host
		}
		out := session.ConnectionConfig{
			ID:     newConnectionID(),
			Name:   name,
			Remark: strings.TrimSpace(conn.Description),
			Type:   typ,
			Host:   host,
			Port:   port,
			User:   user,
		}
		if typ == "ssh" {
			out.AuthType = "password"
		} else if typ == "rdp" {
			if conn.CustomSize && conn.Width > 0 && conn.Height > 0 {
				out.RdpFixedWidth = conn.Width
				out.RdpFixedHeight = conn.Height
			} else if conn.Fullscreen {
				// -1 = follow the primary monitor, FinalShell's fullscreen.
				out.RdpFixedWidth = -1
				out.RdpFixedHeight = -1
			}
		}
		if gid := ensureGroupPath(finalShellGroupPath(conn.ParentID, st.folders), pathMap, &res.Groups, newGroupID); gid != nil {
			out.GroupId = gid
		}
		res.Connections = append(res.Connections, out)
	}
	res.Warnings = st.warns
	return res
}

// finalShellGroupPath walks the parent_id chain up to the root and returns the
// ancestor→leaf group names. Unknown or root parents yield nil (top level).
func finalShellGroupPath(parentID string, folders map[string]finalShellFolder) []string {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" || parentID == "root" || parentID == "0" {
		return nil
	}
	var path []string
	current := parentID
	for range len(folders) + 1 {
		folder, ok := folders[current]
		if !ok {
			return nil
		}
		if name := strings.TrimSpace(folder.Name); name != "" {
			path = append(path, name)
		}
		current = strings.TrimSpace(folder.ParentID)
		if current == "" || current == "root" || current == "0" {
			break
		}
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func displayNameOr(name, host string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return strings.TrimSpace(host)
}
