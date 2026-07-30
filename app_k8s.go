package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/ys-ll/uniterm/backend/k8s"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/session"
)

// ─── Kubernetes ────────────────────────────────────────────────

// K8sContextInfo 是前端可见的 context 元信息。
type K8sContextInfo = k8s.ContextInfo

// K8sListContexts 解析给定 kubeconfig 内容并返回 context 列表。
// source 为文件路径或 YAML 内联字符串；根据 sourceIsPath 区分。
func (a *App) K8sListContexts(source string, sourceIsPath bool) ([]K8sContextInfo, error) {
	raw, err := readKubeconfigSource(source, sourceIsPath)
	if err != nil {
		return nil, err
	}
	return a.k8sManager.ListContexts(raw)
}

// K8sConnect 建立到 kubeconfig 中指定 context 的连接，返回 connID。
// 若 tunnelSSHConnID 非空，会先起一条 SSH 隧道，把到 apiserver 的 TCP 拨号
// 劫持到本地 loopback；TLS 校验仍按 kubeconfig 里的原 hostname 走。
func (a *App) K8sConnect(source string, sourceIsPath bool, contextName string,
	tunnelSSHConnID, tunnelSSHUser, tunnelSSHPassword string) (string, error) {
	raw, err := readKubeconfigSource(source, sourceIsPath)
	if err != nil {
		return "", err
	}

	if tunnelSSHConnID == "" {
		return a.k8sManager.Connect(a.ctx, raw, contextName)
	}

	// ── SSH Tunnel for K8s ─────────────────────────────────────
	if a.tunnelService == nil {
		return "", fmt.Errorf("tunnel service not initialized")
	}
	if a.connectionStore == nil {
		return "", fmt.Errorf("connection store not initialized")
	}
	data, err := a.connectionStore.Load()
	if err != nil {
		return "", fmt.Errorf("load connections for tunnel: %w", err)
	}
	var tunnelSSHConfig *session.ConnectionConfig
	for _, c := range data.Connections {
		if c.ID == tunnelSSHConnID {
			tunnelSSHConfig = &c
			break
		}
	}
	if tunnelSSHConfig == nil {
		return "", fmt.Errorf("tunnel SSH connection not found: %s", tunnelSSHConnID)
	}
	if tunnelSSHUser != "" {
		tunnelSSHConfig.User = tunnelSSHUser
	}
	if tunnelSSHPassword != "" {
		tunnelSSHConfig.Password = tunnelSSHPassword
	}

	// 从 kubeconfig 解出 apiserver host/port 作为隧道目标
	kc, err := k8s.ParseBytes(raw)
	if err != nil {
		return "", fmt.Errorf("kubeconfig: %w", err)
	}
	ctxName := contextName
	if ctxName == "" {
		ctxName = kc.CurrentContext
	}
	ctxEntry, ok := kc.Contexts[ctxName]
	if !ok {
		return "", fmt.Errorf("context %q not found", ctxName)
	}
	cluster, ok := kc.Clusters[ctxEntry.Cluster]
	if !ok {
		return "", fmt.Errorf("cluster %q not found", ctxEntry.Cluster)
	}
	targetHost, targetPort, err := k8s.ParseServerAddr(cluster.Server)
	if err != nil {
		return "", fmt.Errorf("parse apiserver url: %w", err)
	}

	// 用同一个 key 既做 K8s connID 也做 TunnelService 的 sessionID，
	// 方便 Disconnect 时的 onClose 回调直接 Stop 同名隧道。
	tunnelKey := uuid.New().String()
	localPort, err := a.tunnelService.Start(tunnelKey, *tunnelSSHConfig, targetHost, targetPort)
	if err != nil {
		return "", fmt.Errorf("tunnel start: %w", err)
	}
	log.Writef("[K8sConnect] tunnel established for k8s=%s via ssh=%s, localPort=%d",
		tunnelKey, tunnelSSHConnID, localPort)

	var dialer net.Dialer
	dialOverride := func(ctx context.Context, _ /*network*/, _ /*addr*/ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", k8s.LocalAddr(localPort))
	}
	onClose := func() {
		a.tunnelService.Stop(tunnelKey)
	}

	connID, err := a.k8sManager.ConnectWith(raw, contextName, k8s.ConnectOptions{
		PresetID:     tunnelKey,
		DialOverride: dialOverride,
		OnClose:      onClose,
	})
	if err != nil {
		a.tunnelService.Stop(tunnelKey)
		return "", err
	}
	return connID, nil
}

func (a *App) K8sDisconnect(connID string) {
	a.k8sManager.Disconnect(connID)
}

// K8sResponse 是前端可见的 REST 响应。Body 为 JSON 原文字符串。
type K8sResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

func (a *App) K8sRequest(connID, method, path, body, contentType string) (K8sResponse, error) {
	status, out, err := a.k8sManager.Request(a.ctx, connID, method, path, []byte(body), contentType)
	if err != nil {
		return K8sResponse{}, err
	}
	return K8sResponse{Status: status, Body: string(out)}, nil
}

func (a *App) K8sStartWatch(connID, path string) (string, error) {
	return a.k8sManager.StartWatch(connID, path)
}

func (a *App) K8sStopWatch(watchID string) {
	a.k8sManager.StopWatch(watchID)
}

func (a *App) K8sStartLogStream(connID, namespace, pod, container string, tailLines int, timestamps, previous bool) (string, error) {
	return a.k8sManager.StartLogStream(connID, namespace, pod, container, tailLines, timestamps, previous)
}

func (a *App) K8sStopLogStream(streamID string) {
	a.k8sManager.StopLogStream(streamID)
}

func (a *App) K8sExecSession(connID, namespace, pod, container string) (*session.SessionInfo, error) {
	if a.k8sManager == nil {
		return nil, fmt.Errorf("k8s manager not initialized")
	}
	// initial size fallback; real size arrives via Resize after the frontend mounts xterm
	// Use a bounded context so a slow/unreachable apiserver can't hang the
	// WS upgrade indefinitely; cancellation propagates into the dialer.
	dialCtx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	wsConn, err := a.k8sManager.DialExec(dialCtx, connID, namespace, pod, container, 80, 24)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	sess := session.NewK8sExecSession(id, wsConn)
	sess.SetOnDataCallback(func(data []byte) {
		runtime.EventsEmit(a.ctx, "session:data", map[string]interface{}{
			"id":   sess.ID(),
			"data": string(data),
		})
	})
	sess.SetOnStatusChangeCallback(func(status session.SessionStatus) {
		runtime.EventsEmit(a.ctx, "session:status", map[string]interface{}{
			"id":     sess.ID(),
			"status": status,
		})
	})
	a.sessionManager.Add(sess)
	return &session.SessionInfo{ID: id, Type: "k8s-exec", Title: pod, Status: session.StatusConnected}, nil
}

func readKubeconfigSource(source string, sourceIsPath bool) ([]byte, error) {
	if !sourceIsPath {
		return []byte(source), nil
	}
	if len(source) > 1 && source[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			source = filepath.Join(home, source[1:])
		}
	}
	return os.ReadFile(source)
}
