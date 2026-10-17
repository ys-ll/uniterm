package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ys-ll/uniterm/backend/webaccess"
)

// Called by WebTerminalInput under its existing per-session input lock, before
// writing non-binary manual input. Replies/SessionWriteBinary never call this.
// Keep a prepared drop armed until Pick consumes it; a different active writer
// replaces the target. This avoids a target RPC racing another client's input.
func (a *App) webZmodemInputTarget(sessionID, clientID string) error {
	if clientID == "desktop" {
		clientID = ""
	}
	w := &a.webFiles
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("web files closed")
	}
	if w.zTargets == nil {
		w.zTargets = make(map[string]webZmodemTarget)
	}
	target := w.zTargets[sessionID]
	target.clientID, target.stageID = clientID, target.pending[clientID]
	if s := w.stages[target.stageID]; s == nil || s.discard {
		delete(target.pending, clientID)
		target.stageID = ""
	}
	w.zTargets[sessionID] = target
	return nil
}

// The desktop calls this only after its ordered text lane has parsed the final
// binary watermark and restored trailing prompt. All attached observers resync,
// including those which did not initiate the transfer (or picker cancellation).
func (a *App) WebZmodemRestored(ctx context.Context, sessionID string) error {
	if webaccess.IsBrowser(ctx) {
		return webError("native_only", "only the desktop restores ZMODEM terminal output")
	}
	if _, err := a.webTerminalFor(sessionID); err != nil {
		return err
	}
	a.emit("web:zmodem-restored", map[string]string{"sessionId": sessionID})
	return nil
}

// A picker/sink broker only. The native zmodemService remains the sole parser
// and keeps sending binary protocol replies through SessionWriteBinary.
type webZmodemTarget struct {
	clientID, stageID string
	pending           map[string]string // prepared drops; input chooses their owner
}
type WebZmodemChoice struct {
	RequestID string   `json:"requestId"`
	ClientID  string   `json:"clientId"`
	SessionID string   `json:"sessionId"`
	Direction string   `json:"direction"`
	StageID   string   `json:"stageId,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	Directory string   `json:"directory,omitempty"`
}
type webZmodemRequest struct {
	WebZmodemChoice
	answer   chan string
	cancel   context.CancelFunc
	answered bool
}

// A dropped upload is staged before the remote rz command. Preparing a stage
// does not make an observer the target; the locked manual input hook does that.
// Without a stage this is also an explicit native/browser picker selection.
func (a *App) WebZmodemTarget(sessionID, clientID, stageID string) error {
	w := &a.webFiles
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("web files closed")
	}
	if stageID != "" {
		s := w.stages[stageID]
		if s == nil || s.discard || s.State != "ready" {
			return errors.New("upload stage not ready")
		}
		s.SessionID = sessionID
	}
	if w.zTargets == nil {
		w.zTargets = make(map[string]webZmodemTarget)
	}
	target := w.zTargets[sessionID]
	for client, id := range target.pending {
		if s := w.stages[id]; s == nil || s.discard {
			delete(target.pending, client)
		}
	}
	if stageID != "" {
		if target.pending == nil {
			target.pending = make(map[string]string)
		}
		target.pending[clientID] = stageID
	} else {
		target.clientID, target.stageID = clientID, ""
	}
	w.zTargets[sessionID] = target
	return nil
}

// Native parser calls this on detection. A zero choice means use its existing
// desktop dialogs. Web peers filter the request by clientId, never parse bytes.
func (a *App) WebZmodemPick(ctx context.Context, sessionID, direction string) (WebZmodemChoice, error) {
	w := &a.webFiles
	w.mu.Lock()
	target := w.zTargets[sessionID]
	if target.clientID == "" {
		w.mu.Unlock()
		return WebZmodemChoice{}, nil
	}
	if w.closed {
		w.mu.Unlock()
		return WebZmodemChoice{}, errors.New("web files closed")
	}
	for _, req := range w.zmodem {
		if req.SessionID == sessionID {
			w.mu.Unlock()
			return WebZmodemChoice{}, errors.New("ZMODEM already active")
		}
	}
	if direction != "upload" && direction != "download" {
		w.mu.Unlock()
		return WebZmodemChoice{}, errors.New("invalid direction")
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		w.mu.Unlock()
		return WebZmodemChoice{}, err
	}
	id := hex.EncodeToString(b)
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	req := &webZmodemRequest{WebZmodemChoice: WebZmodemChoice{RequestID: id, ClientID: target.clientID, SessionID: sessionID, Direction: direction}, answer: make(chan string, 1), cancel: cancel}
	if w.zmodem == nil {
		w.zmodem = make(map[string]*webZmodemRequest)
	}
	w.zmodem[id] = req
	next := target
	next.stageID = ""
	delete(next.pending, target.clientID)
	w.zTargets[sessionID] = next
	w.mu.Unlock()
	if target.stageID != "" && direction == "upload" {
		req.answer <- target.stageID
	} else {
		a.emit("web:zmodem-pick", req.WebZmodemChoice)
	}
	var stageID string
	select {
	case stageID = <-req.answer:
	case <-waitCtx.Done():
	}
	cancel()
	if stageID == "" {
		_ = a.WebZmodemFinish(id, "cancelled or picker timed out")
		return WebZmodemChoice{}, errors.New("aborted")
	}
	var s *webFileStage
	var err error
	if direction == "download" {
		s, err = a.newWebFileStage("receiving", sessionID)
	} else {
		s, err = a.acquireWebFile(stageID)
	}
	if err != nil {
		_ = a.WebZmodemFinish(id, err.Error())
		return WebZmodemChoice{}, err
	}
	defer a.releaseWebFile(s)
	choice := req.WebZmodemChoice
	choice.StageID = s.ID
	if direction == "download" {
		choice.Directory = filepath.Join(s.dir, "ZMODEM")
		err = os.Mkdir(choice.Directory, 0700)
	} else {
		for _, rel := range a.webFileSnapshot(s).Files {
			p := filepath.Join(s.dir, filepath.FromSlash(rel))
			st, e := os.Stat(p)
			if e != nil {
				err = e
				break
			}
			if st.IsDir() {
				err = errors.New("ZMODEM accepts files, not directories")
				break
			}
			choice.Paths = append(choice.Paths, p)
		}
	}
	w.mu.Lock()
	if req != w.zmodem[id] || w.closed || s.discard || s.active {
		err = errors.New("ZMODEM request expired or stage in use")
	}
	if direction == "upload" && (s.State != "ready" || len(s.Files) == 0 || s.TaskID != "") {
		err = errors.New("ZMODEM requires a completed device upload")
	}
	if err == nil {
		req.WebZmodemChoice = choice
		s.SessionID, s.State, s.active = sessionID, "transferring", true
		if direction == "download" {
			s.target = choice.Directory
		}
	}
	w.mu.Unlock()
	if err != nil {
		a.dismissWebFile(s.ID)
		_ = a.WebZmodemFinish(id, err.Error())
		return WebZmodemChoice{}, err
	}
	return choice, nil
}

func (a *App) WebZmodemAnswer(requestID, clientID, stageID string) error {
	w := &a.webFiles
	w.mu.Lock()
	defer w.mu.Unlock()
	r := w.zmodem[requestID]
	if r == nil || r.ClientID != clientID || r.answered {
		return errors.New("ZMODEM request expired")
	}
	r.answered = true
	r.answer <- stageID // "receive" confirms a browser download; empty cancels
	return nil
}

func (a *App) WebZmodemFinish(requestID, failure string) error {
	w := &a.webFiles
	w.mu.Lock()
	r := w.zmodem[requestID]
	if r == nil {
		w.mu.Unlock()
		return nil
	}
	delete(w.zmodem, requestID)
	r.cancel()
	s := w.stages[r.StageID]
	if s != nil {
		s.active = false
		s.State = "ready"
		if failure != "" {
			s.State, s.Error, s.discard = "error", failure, true
		}
		if r.Direction == "upload" {
			s.discard = true
		}
		a.removeWebFileLocked(s)
	}
	choice := r.WebZmodemChoice
	w.mu.Unlock()
	a.emit("web:zmodem-done", map[string]any{"requestId": requestID, "clientId": choice.ClientID, "sessionId": choice.SessionID, "stageId": choice.StageID, "direction": choice.Direction, "error": failure})
	return nil
}

func (a *App) WebZmodemCancel(sessionID, clientID string) {
	a.webFiles.mu.Lock()
	var requestID string
	for _, r := range a.webFiles.zmodem {
		if r.SessionID == sessionID && r.ClientID == clientID {
			r.cancel()
			requestID = r.RequestID
		}
	}
	a.webFiles.mu.Unlock()
	if requestID != "" {
		a.emit("web:zmodem-cancel", map[string]string{"sessionId": sessionID, "clientId": clientID, "requestId": requestID})
	}
}

func (a *App) WebZmodemProgress(sessionID string, progress map[string]any) {
	a.webFiles.mu.Lock()
	var target string
	for _, r := range a.webFiles.zmodem {
		if r.SessionID == sessionID {
			target = r.ClientID
			if s := a.webFiles.stages[r.StageID]; s != nil {
				s.expires = time.Now().Add(webFileTTL)
			}
			break
		}
	}
	a.webFiles.mu.Unlock()
	if target != "" {
		a.emit("web:zmodem-progress", map[string]any{"clientId": target, "sessionId": sessionID, "transfer": progress})
	}
}
