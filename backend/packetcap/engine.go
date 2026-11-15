package packetcap

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Start begins a capture with the given config and returns a capture id. The
// emitted *:packet events carry this id so the frontend can attribute them.
//
// The engine opens a raw/BPF socket on the platform layer, then runs a capture
// loop that parses each frame, applies the active filters, and emits a packet
// event. The loop runs until Stop(id) is called or a fatal stream error occurs
// — it does NOT terminate itself at MaxPackets. The caller (frontend) decides
// when a capture is done and caps its own display buffer, so a "max packets"
// self-limit would make long captures stop unpredictably (as soon as a fast
// network reaches the cap). A final "stopped" event is always emitted.
func (m *Manager) Start(cfg CaptureConfig) (string, error) {
	if cfg.Remote {
		if cfg.SSHConfig == nil || cfg.SSHConfig.Host == "" {
			return "", fmt.Errorf("no remote host resolved for capture")
		}
		if cfg.RemoteConnID == "" {
			return "", fmt.Errorf("no remote connection selected")
		}
	} else if cfg.InterfaceIP == "" {
		return "", fmt.Errorf("no interface IP selected")
	}
	f := ParseFilter(cfg.Filter)

	ctx, cancel := context.WithCancel(context.Background())
	id := newCaptureID()
	sess := &captureSession{cancel: cancel}
	m.mu.Lock()
	m.captures[id] = sess
	m.mu.Unlock()

	go m.captureLoop(ctx, cancel, id, cfg, f)
	return id, nil
}

func (m *Manager) captureLoop(ctx context.Context, cancel context.CancelFunc, id string, cfg CaptureConfig, f *Filter) {
	stopReason := "manual"
	count := 0

	// The platform capture is a callback stream: onPacket is invoked for each
	// parsed packet. Keeping filter/emit/count here leaves the platform layer
	// (raw socket / elevated worker) uniform across OSes.
	onPacket := func(pkt *Packet) {
		if ctx.Err() != nil || pkt == nil {
			return
		}
		if !f.Active(pkt) {
			return
		}
		count++
		m.emit(EventPacket, map[string]any{"captureId": id, "packet": pkt})
	}
	errCh := make(chan error, 1)
	go func() {
		// Remote capture runs tcpdump over SSH and feeds frames into the same
		// onPacket pipeline (parse/filter/emit); local capture opens a raw socket.
		if cfg.Remote {
			errCh <- remoteCapture(ctx, cfg, onPacket)
			return
		}
		errCh <- platformCapturePackets(ctx, cfg, onPacket)
	}()

	var streamErr error
	select {
	case <-ctx.Done():
		// user stopped
		log.Printf("pktcap captureLoop id=%q: ctx done (stop)", id)
	case streamErr = <-errCh:
		log.Printf("pktcap captureLoop id=%q: stream returned err=%v", id, streamErr)
	}

	if streamErr != nil && streamErr != context.Canceled && streamErr != errStopped {
		stopReason = "error"
	}
	log.Printf("pktcap captureLoop id=%q: emitting stopped reason=%q count=%d streamErr=%v", id, stopReason, count, streamErr)
	m.emit(EventStopped, map[string]any{
		"captureId": id,
		"reason":    stopReason,
		"count":     count,
	})
	// Deregister so a later Stop(id) is a no-op.
	m.Stop(id)
}

func newCaptureID() string {
	return fmt.Sprintf("cap-%d", time.Now().UnixNano())
}
