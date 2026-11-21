package session

// Typed cwd-hook injection for LocalSession, shared by the Windows (WSL +
// local shells) and Unix builds. The hook state machine lives in
// shell_integration.go; the per-platform struct definitions carry the fields.

import (
	"fmt"
	"strings"
	"time"

	"github.com/ys-ll/uniterm/backend/log"
)

// wslCwdHookRowCleanup is prepended to the hook body (phase 2, echo off) to
// wipe the echo of phase 1: save the cursor, clear the home row, restore.
// Absolute positioning, so it cannot drift with the prompt layout. The wsl
// prefix is historical (it was built for WSL conhost); the sequence is
// plain ANSI and works on every terminal.
const wslCwdHookRowCleanup = " printf '\\0337\\033[H\\033[2K\\0338';"

// InjectCwdHook types the cwd-hook on demand — invoked when the user enables
// directory follow for this session (with an explicit confirmation in the
// UI). Never called at session start: a plain first login must show no
// handshake output at all (issue #1388), and cross-shell cleanup differences
// are the user's informed choice to accept, not a startup constant. The
// write waits briefly for in-flight output to settle so the typed lines land
// at a prompt.
func (s *LocalSession) InjectCwdHook() error {
	if s.cwdHookBody == "" {
		return fmt.Errorf("directory follow is unavailable for this shell (%s has no prompt hook support; an OSC-7 prompt integration would work instead)", s.foregroundShell)
	}
	if s.hook.isInstalled() || !s.cwdHookRequested.CompareAndSwap(false, true) {
		return nil
	}
	go func() {
		if !s.waitIdle(3*time.Second, 300*time.Millisecond) {
			log.Writef("local: no idle window for cwd hook, injecting anyway")
		}
		if s.Status() != StatusConnected {
			return
		}
		if s.cwdHookShellBase == "bash" || s.cwdHookShellBase == "zsh" {
			// The body carries the row cleanup with it: the arm line's echo
			// must be erased by the time the body runs (echo off).
			s.hook.arm(wslCwdHookRowCleanup + s.cwdHookBody)
			_ = s.Write([]byte(cwdHookArmLine))
			// Bound the arm phase too: if the echo-armed marker never arrives
			// (shell died mid-startup, exotic setup), nothing else would
			// restore the echo the arm line just turned off.
			go s.watchCwdHookConfirm()
			return
		}
		// fish, PowerShell and other single-phase shells without an echo-off
		// prefix: write the body directly and watch for the ready marker.
		// PowerShell's PSReadLine only treats \r as accept-line — a trailing
		// \n would leave the snippet sitting at the prompt until the user
		// presses Enter manually.
		body := s.cwdHookBody
		if s.cwdHookShellBase == "powershell" || s.cwdHookShellBase == "pwsh" {
			body = strings.TrimSuffix(body, "\n") + "\r"
		}
		if err := s.Write([]byte(body)); err != nil {
			log.Writef("local: cwd hook write failed: %v", err)
			return
		}
		log.Writef("local: cwd hook injected (shell=%s)", s.cwdHookShellBase)
		go s.watchCwdHookConfirm()
	}()
	return nil
}

// writeCwdHookBody is phase 2: runs once the echo-armed marker confirmed that
// stty -echo took effect, so the hook body cannot echo. The body already
// carries the home-row cleanup that erases the arm line's echo (it is
// prepended when the hook is armed).
func (s *LocalSession) writeCwdHookBody(body string) {
	if body == "" {
		return
	}
	if err := s.Write([]byte(body)); err != nil {
		log.Writef("local: cwd hook body write failed: %v", err)
		return
	}
	log.Writef("local: cwd hook injected (shell=%s)", s.cwdHookShellBase)
	go s.watchCwdHookConfirm()
}

// watchCwdHookConfirm bounds how long the session waits for the injected
// hook's ready marker. If the hook never confirms (unsupported setup, the
// user exited the shell instantly), echo is restored blindly so the session
// is never left with terminal echo off. The give-up is recorded exactly once
// (a watchdog runs for the arm phase and another for the body) so settle()
// waiters see a stable terminal state.
func (s *LocalSession) watchCwdHookConfirm() {
	select {
	case <-time.After(cwdHookConfirmTimeout):
	case <-s.quit:
		return
	}
	if s.hook.isInstalled() || s.Status() != StatusConnected {
		return
	}
	if !s.hook.giveUp.CompareAndSwap(false, true) {
		return
	}
	log.Writef("local: cwd hook not confirmed after %s, restoring echo blindly", cwdHookConfirmTimeout)
	_ = s.Write([]byte(cwdHookBlindRestoreLine))
}
