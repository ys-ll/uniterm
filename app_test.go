package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/credentials"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
	unitsync "github.com/ys-ll/uniterm/backend/sync"
	"sync"
	"testing"
	"time"
)

// TestApp_StartupError_CapturesSingleError verifies that a single error
// sent during startup() surfaces through StartupError() after
// drainStartupErr() has run. The frontend calls StartupError() on demand
// after receiving the "app:startup-error" event to render a banner.
func TestApp_StartupError_CapturesSingleError(t *testing.T) {
	a := NewApp("")
	a.sendStartupErr(errors.New("connection store: init failed"))

	if got := a.StartupError(); got != "" {
		t.Fatalf("StartupError() before drain = %q, want empty", got)
	}

	a.drainStartupErr()
	got := a.StartupError()
	want := "connection store: init failed"
	if got != want {
		t.Fatalf("StartupError() = %q, want %q", got, want)
	}
}

// TestApp_StartupError_JoinsMultipleErrors verifies the errors.Join
// behaviour: multiple sendStartupErr calls during startup() produce a
// single newline-joined string readable by the frontend.
func TestApp_StartupError_JoinsMultipleErrors(t *testing.T) {
	a := NewApp("")
	a.sendStartupErr(errors.New("connection store: read failed"))
	a.sendStartupErr(errors.New("settings store: write failed"))
	a.drainStartupErr()

	got := a.StartupError()
	for _, want := range []string{"connection store: read failed", "settings store: write failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("StartupError() = %q, missing %q", got, want)
		}
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("StartupError() = %q, expected newline-separated entries", got)
	}
}

// TestApp_StartupError_NilOnClean verifies StartupError() returns ""
// when no init failures occurred. The frontend uses the empty string to
// decide whether to render the banner.
func TestApp_StartupError_NilOnClean(t *testing.T) {
	a := NewApp("")
	a.drainStartupErr()
	if got := a.StartupError(); got != "" {
		t.Fatalf("StartupError() with no errors = %q, want empty", got)
	}
}

// TestApp_SendStartupErr_NilIsNoop confirms nil errors do not enqueue.
// Defensive against a future caller passing nil from an error chain.
func TestApp_SendStartupErr_NilIsNoop(t *testing.T) {
	a := NewApp("")
	a.sendStartupErr(nil)
	a.drainStartupErr()
	if got := a.StartupError(); got != "" {
		t.Fatalf("StartupError() after nil send = %q, want empty", got)
	}
}

// TestApp_SendStartupErr_OverflowDrops verifies that when the buffered
// channel (size 16) overflows, sendStartupErr drops the extra error
// instead of blocking. The send runs on the startup goroutine and must
// remain non-blocking; the log line is the last-resort record.
func TestApp_SendStartupErr_OverflowDrops(t *testing.T) {
	a := NewApp("")
	for i := 0; i < 32; i++ {
		a.sendStartupErr(fmt.Errorf("store: %d", i))
	}
	a.drainStartupErr()
	got := a.StartupError()
	if got == "" {
		t.Fatal("StartupError() empty after 32 sends, want at least one entry")
	}
	if strings.Count(got, "\n") >= 32 {
		t.Errorf("StartupError() kept all 32 sends, expected some drops under cap=16")
	}
}

// TestApp_StartupErr_ConcurrentSendSafe sends from many goroutines and
// confirms drainStartupErr() collects everything safely. Catches data
// races in the channel/snapshot interaction under -race.
func TestApp_StartupErr_ConcurrentSendSafe(t *testing.T) {
	a := NewApp("")
	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			a.sendStartupErr(fmt.Errorf("concurrent: %d", i))
		}(i)
	}
	wg.Wait()
	a.drainStartupErr()
	got := a.StartupError()
	if got == "" {
		t.Fatal("StartupError() empty after concurrent sends")
	}
}

// TestApp_PanelLogTitle_FallbackAndLookup exercises panelLogTitle: with
// no registered session the helper returns a synthetic fallback rather
// than crashing on a nil sessionManager, and with a registered session
// it returns non-empty name/protocol under panelLogMu. Regression
// coverage for the helper that PR-15's log-file work depends on.
func TestApp_PanelLogTitle_FallbackAndLookup(t *testing.T) {
	a := NewApp("")

	// No registration — fallback path.
	name, _, protocol := a.panelLogTitle("panel-X")
	if name == "" {
		t.Errorf("panelLogTitle empty name for unregistered panel")
	}
	if protocol == "" {
		t.Errorf("panelLogTitle empty protocol for unregistered panel")
	}

	// Registered session but no live sessionManager entry — helper
	// falls through to the synthetic name (no panic).
	a.RegisterSessionForPanel("sess-y", "panel-X")
	start := time.Now()
	for i := 0; i < 1000; i++ {
		name, _, protocol = a.panelLogTitle("panel-X")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("1000 panelLogTitle calls took %v, expected < 50ms", elapsed)
	}
	if name == "" || protocol == "" {
		t.Errorf("panelLogTitle returned empty name=%q protocol=%q", name, protocol)
	}
}

// TestErrorBodyCap_F305 guards F-305: the error-body read on the non-200
// path is capped at 64 KiB so a hostile or buggy upstream returning a
// multi-GB error body can't OOM the Go process. We can't drive the full
// chatCompletion* HTTP path from a unit test, but the cap is a single
// io.LimitReader wrapper around io.ReadAll — verify the contract that
// reads beyond 64 KiB stop there.
func TestErrorBodyCap_F305(t *testing.T) {
	const cap = 64 * 1024
	// 1 MiB of 'a' is well above the cap.
	big := strings.Repeat("a", 1024*1024)
	body, err := io.ReadAll(io.LimitReader(strings.NewReader(big), cap))
	if err != nil {
		t.Fatalf("LimitReader read failed: %v", err)
	}
	if len(body) != cap {
		t.Errorf("expected capped read of %d bytes, got %d", cap, len(body))
	}
	// Sanity: a smaller body is read in full.
	small := strings.Repeat("b", 1024)
	bodySmall, err := io.ReadAll(io.LimitReader(strings.NewReader(small), cap))
	if err != nil {
		t.Fatalf("LimitReader read failed: %v", err)
	}
	if len(bodySmall) != len(small) {
		t.Errorf("small body got %d bytes, want %d", len(bodySmall), len(small))
	}
}

// TestApp_CancelChatStream_NoActiveCall guards the safe-no-op path:
// when no ChatCompletion is in flight, CancelChatStream must not panic.
func TestApp_CancelChatStream_NoActiveCall(t *testing.T) {
	a := &App{}
	a.CancelChatStream() // must not panic
}

// TestApp_CancelChatStream_OverlappingCalls guards F-308: when two
// ChatCompletion calls overlap, CancelChatStream invoked during the second
// call must still cancel the second call's context — not be silently
// no-op'd because the first call's defer already nil'd the slot.
func TestApp_CancelChatStream_OverlappingCalls(t *testing.T) {
	a := &App{}

	_, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	myA := cancelA
	a.chatCancel.Store(&myA)

	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	myB := cancelB
	a.chatCancel.Store(&myB)

	// Call A finishes first — its defer must NOT clobber B's slot.
	a.chatCancel.CompareAndSwap(&myA, nil)

	a.CancelChatStream()

	if ctxB.Err() == nil {
		t.Errorf("ctxB expected to be cancelled by CancelChatStream")
	}
}

// TestApp_ConcurrentCancelSurvives guards F-308's race at scale: the
// old mutex+CancelFunc design could let a CancelChatStream drop a
// cancel because the wrong defer cleared the slot. With
// atomic.Pointer[CancelFunc] + CompareAndSwap(my, nil), only the call
// that currently owns the slot ever clears it — the rest leave it
// alone. This test verifies the simpler invariant: a call's cancel
// survives any number of OTHER calls' defers running first.
func TestApp_ConcurrentCancelSurvives(t *testing.T) {
	a := &App{}

	// Two overlapping calls.
	_, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	_, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	myA := cancelA
	myB := cancelB

	a.chatCancel.Store(&myA)
	a.chatCancel.Store(&myB)

	// N goroutines, each simulating a call whose defer tries to clear
	// the slot. None of them is "call A" or "call B" — their CAS
	// addresses don't match anything in the slot, so they all no-op.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := context.CancelFunc(func() {})
			a.chatCancel.CompareAndSwap(&other, nil)
		}()
	}
	wg.Wait()

	// After all the unrelated defers, the slot must still hold B's cancel
	// — the no-op CASes never touched it.
	c := a.chatCancel.Load()
	if c == nil {
		t.Fatalf("chatCancel unexpectedly nil; B's cancel was wiped by a stale defer")
	}
	if c != &myB {
		t.Errorf("chatCancel points at %p, want B's cancel at %p", c, &myB)
	}
}

// TestSetDataDir_RejectsSameDir guards the migration self-copy: choosing the
// directory the app already runs from must be refused before anything is
// copied or removed — copyDir onto itself followed by removeDataDir would
// delete the only copy of the data.
func TestSetDataDir_RejectsSameDir(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "connections.json")
	if err := os.WriteFile(seed, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}

	a := &App{dataDir: dir}
	if err := a.SetDataDir("custom", dir, true); err == nil {
		t.Fatal("migrating onto the current data dir must be rejected")
	}
	if _, err := os.Stat(seed); err != nil {
		t.Fatalf("current data dir must be untouched: %v", err)
	}
}

// TestSetDataDir_RejectsDirInsideCurrent: a target inside the current data
// dir is walked by copyDir while it is being written and then removed by
// removeDataDir together with the source — data loss either way.
func TestSetDataDir_RejectsDirInsideCurrent(t *testing.T) {
	parent := t.TempDir()
	seed := filepath.Join(parent, "connections.json")
	if err := os.WriteFile(seed, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(parent, "inside")

	a := &App{dataDir: parent}
	if err := a.SetDataDir("custom", inside, true); err == nil {
		t.Fatal("migrating into the current data dir must be rejected")
	}
	if _, err := os.Stat(seed); err != nil {
		t.Fatalf("current data dir must be untouched: %v", err)
	}
}

// TestResetCredentialsAllowsFreshSetup is the reset-wedge regression: after a
// reset, no in-place ciphertext may remain in any manifest-declared file —
// Setup refuses a data dir that still carries it, which used to leave the
// user unable to configure a new master password (the old hand-written clear
// list missed key content and ai.json).
func TestResetCredentialsAllowsFreshSetup(t *testing.T) {
	dir := t.TempDir()
	seed := map[string]string{
		"connections.json": `{"connections":[{"id":"c1","password":"enc:v1:a","keyContent":"enc:v1:b"}]}`,
		"identities.json":  `{"identities":[{"id":"i1","password":"enc:v1:c","keyContent":"enc:v1:d"}]}`,
		"proxies.json":     `{"proxies":[{"id":"p1","pass":"enc:v1:e"}]}`,
		"ai.json":          `{"models":[{"id":"m1","apiKey":"enc:v1:f"}]}`,
		"settings.json":    `{"ai":{"models":[{"id":"m2","apiKey":"enc:v1:g"}]}}`,
	}
	for name, body := range seed {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}

	files := make([]string, 0, len(store.SecretFiles))
	for _, sf := range store.SecretFiles {
		files = append(files, sf.File)
	}
	cs := credentials.New(dir, memKeychain{entries: map[string]string{}})
	cs.SetSecretFiles(files)
	a := &App{dataDir: dir, credentialStore: cs}

	if err := a.ResetCredentials(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := cs.Setup(credentials.ModeMasterPassword, "new-pw"); err != nil {
		t.Fatalf("setup after reset must succeed: %v", err)
	}
}

// memKeychain is an in-memory credentials.Keychain for tests that must not
// touch the OS keyring.
type memKeychain struct{ entries map[string]string }

func (k memKeychain) Get(key string) (string, error) {
	v, ok := k.entries[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (k memKeychain) Set(key, val string) error { k.entries[key] = val; return nil }
func (k memKeychain) Delete(key string) error   { delete(k.entries, key); return nil }
func (k memKeychain) List() ([]string, error) {
	keys := make([]string, 0, len(k.entries))
	for name := range k.entries {
		keys = append(keys, name)
	}
	return keys, nil
}

// --- Rekey regression: reencryptAll's saves used to be swallowed by the
// F-105 hash-skip — Load seeds lastSavedHash with the plaintext content
// hash, the rekey Save serializes the same content, and the write is
// skipped, leaving every file under the old key while meta already points
// at the new one. Restart then rejects even the correct password
// (verification decrypts a field still under the old key) — an
// unrecoverable lockout. ai.json was additionally missing from the
// reencrypt list entirely. Every manifest secret file must decrypt under
// the new key after any rekey. ---

type rekeyHarness struct {
	dir           string
	kc            memKeychain
	cs            *credentials.Store
	connStore     *store.ConnectionStore
	settingsStore *store.SettingsStore
	identStore    *store.IdentityStore
	proxyStore    *store.ProxyStore
	aiStore       *store.AIConfigStore
	mcpStore      *store.MCPClientStore
	tunnelStore   *store.TunnelStore
}

func newRekeyHarness(t *testing.T, startMode, startPass string) rekeyHarness {
	t.Helper()
	dir := t.TempDir()
	kc := memKeychain{entries: map[string]string{}}
	cs := credentials.New(dir, kc)
	files := make([]string, 0, len(store.SecretFiles))
	for _, sf := range store.SecretFiles {
		files = append(files, sf.File)
	}
	cs.SetSecretFiles(files)
	if err := cs.Setup(startMode, startPass); err != nil {
		t.Fatal(err)
	}

	connStore, err := store.NewConnectionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := store.NewSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	identStore, err := store.NewIdentityStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	proxyStore, err := store.NewProxyStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	aiStore, err := store.NewAIConfigStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	connStore.SetPasswordStore(cs)
	settingsStore.SetPasswordStore(cs)
	identStore.SetPasswordStore(cs)
	proxyStore.SetPasswordStore(cs)
	aiStore.SetPasswordStore(cs)
	mcpStore := store.NewMCPClientStore(dir)
	mcpStore.SetPasswordStore(cs)
	tunnelStore := store.NewTunnelStore(dir)
	tunnelStore.SetPasswordStore(cs)

	h := rekeyHarness{dir: dir, kc: kc, cs: cs, connStore: connStore,
		settingsStore: settingsStore, identStore: identStore,
		proxyStore: proxyStore, aiStore: aiStore, mcpStore: mcpStore,
		tunnelStore: tunnelStore}
	// Seed one encrypted secret into every manifest file.
	if err := connStore.Save(session.ConnectionStoreData{Connections: []session.ConnectionConfig{{
		ID: "c1", Type: "ssh", Host: "h", Port: 22, AuthType: "password", Password: "pw-conn"}}}); err != nil {
		t.Fatal(err)
	}
	if err := identStore.Save(session.IdentityStoreData{Identities: []session.Identity{{
		ID: "i1", AuthType: "password", Password: "pw-ident"}}}); err != nil {
		t.Fatal(err)
	}
	if err := proxyStore.Save(session.ProxyStoreData{Proxies: []session.Proxy{{
		ID: "px1", Kind: "socks5", Host: "127.0.0.1", Port: 1080, Pass: "pw-proxy"}}}); err != nil {
		t.Fatal(err)
	}
	if err := aiStore.Save(store.AIStoreData{Models: []store.AIModelConfig{{
		ID: "m1", Name: "n", APIKey: "pw-ai", BaseURL: "http://x", Model: "m", Protocol: "openai"}}}); err != nil {
		t.Fatal(err)
	}
	if err := settingsStore.Save(store.AppSettings{AI: store.AISettings{Models: []store.AIModelConfig{{
		ID: "m2", Name: "s", APIKey: "pw-ai-settings"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := mcpStore.Save([]store.MCPClientConfig{{Name: "external", Transport: store.MCPTransportStreamableHTTP, URL: "http://localhost/mcp", Headers: map[string]string{"Authorization": "pw-mcp"}}}); err != nil {
		t.Fatal(err)
	}
	if err := tunnelStore.Save(session.TunnelStoreData{Tunnels: []session.Tunnel{{
		ID: "t1", Name: "tunnel-1", Mode: session.TunnelLocal, SSHConnID: "c1",
		ListenHost: "127.0.0.1", ListenPort: 1081,
		Upstream: &session.SocksProxy{Host: "127.0.0.1", Port: 1080, Pass: "pw-tunnel"},
	}}}); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h rekeyHarness) app() *App {
	return &App{dataDir: h.dir, credentialStore: h.cs,
		connectionStore: h.connStore, settingsStore: h.settingsStore,
		identityStore: h.identStore, proxyStore: h.proxyStore,
		aiConfigStore: h.aiStore, mcpClientStore: h.mcpStore,
		tunnelStore: h.tunnelStore}
}

// assertAllFilesDecryptUnderKey reads every manifest secret file and checks
// its first enc:v1 value opens with key — the observable of a completed
// rekey (a stale file fails here even when meta/keychain were updated).
func assertAllFilesDecryptUnderKey(t *testing.T, dir string, files []string, key []byte) {
	t.Helper()
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			if os.IsNotExist(err) {
				// A manifest file that was never written holds no secrets —
				// every manifest consumer (reset blanking, secret counting)
				// treats a missing file the same way. sync-keychain.json only
				// exists on server builds once sync is configured.
				continue
			}
			t.Fatalf("%s: %v", f, err)
		}
		s := string(raw)
		i := strings.Index(s, credentials.Prefix)
		if i < 0 {
			t.Fatalf("%s: no ciphertext present", f)
		}
		val := s[i:]
		if j := strings.Index(val, `"`); j >= 0 {
			val = val[:j]
		}
		if _, err := credentials.DecryptField(val, key); err != nil {
			t.Fatalf("%s: ciphertext does not decrypt under the new key (rekey wrote nothing): %v", f, err)
		}
	}
}

func rekeyRestart(t *testing.T, h rekeyHarness) *credentials.Store {
	t.Helper()
	files := make([]string, 0, len(store.SecretFiles))
	for _, sf := range store.SecretFiles {
		files = append(files, sf.File)
	}
	cs2 := credentials.New(h.dir, h.kc)
	cs2.SetSecretFiles(files)
	if err := cs2.AutoUnlock(); err != nil {
		t.Fatalf("restart AutoUnlock: %v", err)
	}
	if !cs2.Status().Unlocked {
		t.Fatalf("restart must auto-unlock after rekey: %+v", cs2.Status())
	}
	return cs2
}

func TestSwitchCredentialModeReencryptsEverySecretFile(t *testing.T) {
	h := newRekeyHarness(t, credentials.ModeKeychain, "")
	if err := h.app().SwitchCredentialMode(credentials.ModeMasterPassword, "new-pass"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	meta, err := credentials.ReadMeta(h.dir)
	if err != nil || meta == nil {
		t.Fatalf("read meta: %v %v", meta, err)
	}
	newKey := unitsync.DeriveKey("new-pass", meta.Salt)
	files := make([]string, 0, len(store.SecretFiles))
	for _, sf := range store.SecretFiles {
		files = append(files, sf.File)
	}
	assertAllFilesDecryptUnderKey(t, h.dir, files, newKey)
	rekeyRestart(t, h)
}

func TestChangeMasterPasswordReencryptsEverySecretFile(t *testing.T) {
	h := newRekeyHarness(t, credentials.ModeMasterPassword, "old-pass")
	if err := h.app().ChangeMasterPassword("old-pass", "new-pass"); err != nil {
		t.Fatalf("change: %v", err)
	}
	meta, err := credentials.ReadMeta(h.dir)
	if err != nil || meta == nil {
		t.Fatalf("read meta: %v %v", meta, err)
	}
	newKey := unitsync.DeriveKey("new-pass", meta.Salt)
	files := make([]string, 0, len(store.SecretFiles))
	for _, sf := range store.SecretFiles {
		files = append(files, sf.File)
	}
	assertAllFilesDecryptUnderKey(t, h.dir, files, newKey)
	rekeyRestart(t, h)
}

// TestNormalizeLanguageCode covers the locale-tag matching used to localize
// the tray menu (issue #1215): it must mirror the frontend's resolveLocale —
// zh variants split between Simplified and Traditional, everything else
// matches by primary language subtag, unknown tags fall back to English.
func TestNormalizeLanguageCode(t *testing.T) {
	cases := map[string]string{
		// Exact supported codes (any case).
		"zh-CN": "zh-CN",
		"zh-TW": "zh-TW",
		"en":    "en",
		"ja":    "ja",
		"vi":    "vi",
		// Region-qualified tags match by primary subtag.
		"en-US": "en",
		"en-GB": "en",
		"pt-BR": "pt",
		"pt-PT": "pt",
		"de-AT": "de",
		"fr-CA": "fr",
		"ko-KR": "ko",
		"ru-RU": "ru",
		"es-MX": "es",
		"ja-JP": "ja",
		// Underscore form (Linux/macOS locale style) and encoding suffix.
		"en_US":       "en",
		"zh_CN.UTF-8": "zh-CN",
		"zh_TW.utf8":  "zh-TW",
		"zh_HK@rg":    "zh-TW",
		// Script subtags decide the Chinese written form.
		"zh-Hans-CN": "zh-CN",
		"zh-Hant-TW": "zh-TW",
		// Unmapped tags fall back to English.
		"xx":    "en",
		"xx-YY": "en",
		"":      "en",
	}
	for in, want := range cases {
		if got := normalizeLanguageCode(in); got != want {
			t.Errorf("normalizeLanguageCode(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolveTrayLanguage checks the explicit-language path; the "system"
// path calls into platform code and depends on the host locale, so it is not
// asserted here beyond resolving to a trayLabels key.
func TestResolveTrayLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"zh-CN": "zh-CN",
		"en":    "en",
		"pt-BR": "pt",
		"xx-YY": "en",
	} {
		if got := resolveTrayLanguage(in); got != want {
			t.Errorf("resolveTrayLanguage(%q) = %q, want %q", in, got, want)
		}
	}
	if _, ok := trayLabels[resolveTrayLanguage("system")]; !ok {
		t.Errorf("resolveTrayLanguage(%q) = %q, want a trayLabels key", "system", resolveTrayLanguage("system"))
	}
}

// TestWindowGeometryVisible_Offscreen: a rect saved on a monitor that is no
// longer attached must be rejected so startup falls back to centered instead
// of restoring the window somewhere invisible (issue #1335).
func TestWindowGeometryVisible_Offscreen(t *testing.T) {
	screens := []*application.Screen{
		{WorkArea: application.Rect{X: 0, Y: 0, Width: 1920, Height: 1080}},
	}
	if windowGeometryVisible(2200, 100, 1200, 800, screens) {
		t.Error("rect entirely right of the only screen reported visible")
	}
	if windowGeometryVisible(100, 100, 0, 800, screens) {
		t.Error("zero-width rect reported visible")
	}
}

// TestWindowGeometryVisible_OnScreen: a saved position on an attached screen
// keeps its exact coordinates.
func TestWindowGeometryVisible_OnScreen(t *testing.T) {
	screens := []*application.Screen{
		{WorkArea: application.Rect{X: 0, Y: 0, Width: 1920, Height: 1080}},
	}
	if !windowGeometryVisible(300, 200, 1200, 800, screens) {
		t.Error("rect inside the only screen reported invisible")
	}
}

// TestWindowGeometryVisible_NegativeCoordinates: monitors left of or above the
// primary have negative work-area coordinates; a visible rect there is valid.
func TestWindowGeometryVisible_NegativeCoordinates(t *testing.T) {
	screens := []*application.Screen{
		{WorkArea: application.Rect{X: -1920, Y: 0, Width: 1920, Height: 1080}},
	}
	if !windowGeometryVisible(-1600, 100, 1200, 800, screens) {
		t.Error("rect on a monitor left of the primary reported invisible")
	}
}

// TestWindowGeometryVisible_PartialOverlap: a window poking onto a screen by at
// least the 100x50 grab area stays; a thinner sliver is rejected.
func TestWindowGeometryVisible_PartialOverlap(t *testing.T) {
	screens := []*application.Screen{
		{WorkArea: application.Rect{X: 0, Y: 0, Width: 1920, Height: 1080}},
	}
	if !windowGeometryVisible(1820, 1030, 1200, 800, screens) {
		t.Error("rect overlapping the screen by exactly 100x50 reported invisible")
	}
	if windowGeometryVisible(1821, 1030, 1200, 800, screens) {
		t.Error("rect overlapping the screen by only 99x50 reported visible")
	}
}

// TestWindowGeometryVisible_NoScreens: platforms that enumerate no screens
// (headless/server) keep the old behavior — accept the saved rect, never
// strand the window.
func TestWindowGeometryVisible_NoScreens(t *testing.T) {
	if !windowGeometryVisible(5000, 5000, 1200, 800, nil) {
		t.Error("nil screen list reported invisible; want old behavior preserved")
	}
}
