package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/ys-ll/uniterm/backend/container"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
)

func TestBusinessSettingsStaleAndConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ai, err := store.NewAIConfigStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{settingsStore: s, aiConfigStore: ai}
	snapshot, err := a.LoadBusinessConfig("settings")
	if err != nil {
		t.Fatal(err)
	}
	var settings store.AppSettings
	if err := json.Unmarshal([]byte(snapshot.JSON), &settings); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, theme := range []string{"light", "deep-blue"} {
		wg.Add(1)
		go func(theme string) {
			defer wg.Done()
			copy := settings
			copy.Theme = theme
			raw, _ := json.Marshal(copy)
			_, err := a.SaveBusinessConfig("settings", snapshot.Revision, string(raw))
			results <- err
		}(theme)
	}
	wg.Wait()
	close(results)
	passed, conflicted := 0, 0
	for err := range results {
		if err == nil {
			passed++
		} else if errors.Is(err, store.ErrConflict) {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}
	if passed != 1 || conflicted != 1 {
		t.Fatalf("saved=%d conflicts=%d", passed, conflicted)
	}
	latest, err := a.LoadBusinessConfig("settings")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.SaveBusinessConfig("settings", snapshot.Revision, snapshot.JSON); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	still, _ := a.LoadBusinessConfig("settings")
	if still != latest {
		t.Fatal("stale settings replaced latest config")
	}
	// An external/sync store write is detected by content, not only RPC counters.
	cfg, _ := ai.Load()
	cfg.Models = []store.AIModelConfig{{ID: "remote", Model: "new"}}
	if err := ai.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveBusinessConfig("settings", latest.Revision, latest.JSON); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("sync AI changes overwritten: %v", err)
	}
}

func TestContainerRuntimeHandlesAreIndependent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runtime fixture uses a POSIX shell")
	}
	// Exercise the actual connect RPC without a daemon: validation uses this
	// tiny runtime executable, while each connection still creates a real runner.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nprintf 'Docker version test\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	connections, err := store.NewConnectionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := connections.Save(session.ConnectionStoreData{Connections: []session.ConnectionConfig{{ID: "saved", Type: "container", ContainerRuntime: "docker", ContainerTransport: "local"}}}); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), connectionStore: connections, containerManager: container.NewManager()}
	first, err := a.ContainerConnect("saved", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.ContainerConnect("saved", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer a.ContainerDisconnect(second)
	if first == second || first == "saved" || second == "saved" {
		t.Fatal("saved ID reused as a runtime handle")
	}
	release, err := a.containerManager.Retain(first)
	if err != nil {
		t.Fatal(err)
	}
	closed := 0
	a.containerManager.SetOnClose(first, func() { closed++ })
	a.ContainerDisconnect(first)
	if closed != 0 {
		t.Fatal("page close stopped the terminal's tunnel")
	}
	if _, err := a.containerManager.Provider(first); err != nil {
		t.Fatalf("page close killed terminal runner: %v", err)
	}
	release()
	release()
	if closed != 1 {
		t.Fatalf("tunnel cleanup count = %d", closed)
	}
	if _, err := a.containerManager.Provider(first); err == nil {
		t.Fatal("runner leaked after final terminal close")
	}
	if _, err := a.containerManager.Provider(second); err != nil {
		t.Fatalf("other browser disconnected: %v", err)
	}
}
