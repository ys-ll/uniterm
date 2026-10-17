package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
	cloudsync "github.com/ys-ll/uniterm/backend/sync"
)

// BusinessConfig is an RPC envelope only: existing config files keep their
// format. Hashes cover freshly loaded content, including changes from sync.
type BusinessConfig struct {
	Revision string `json:"revision"`
	JSON     string `json:"json"`
}

func configEnvelope(data any, err error) (BusinessConfig, error) {
	if err != nil {
		return BusinessConfig{}, err
	}
	revision, err := store.Revision(data)
	if err != nil {
		return BusinessConfig{}, err
	}
	raw, err := json.Marshal(data)
	return BusinessConfig{Revision: revision, JSON: string(raw)}, err
}

func (a *App) LoadBusinessConfig(kind string) (BusinessConfig, error) {
	a.businessMu.Lock()
	defer a.businessMu.Unlock()
	return a.loadBusinessConfig(kind)
}

func (a *App) loadBusinessConfig(kind string) (BusinessConfig, error) {
	switch kind {
	case "sync":
		return configEnvelope(a.SyncGetConfig())
	case "connections":
		return configEnvelope(a.LoadConnections())
	case "identities":
		return configEnvelope(a.LoadIdentities())
	case "proxies":
		return configEnvelope(a.LoadProxies())
	case "tunnels":
		return configEnvelope(a.LoadTunnels())
	case "localState":
		return configEnvelope(a.LoadLocalState())
	case "ai":
		return configEnvelope(a.LoadAIModels())
	case "quickCommands":
		return configEnvelope(a.LoadQuickCommands())
	case "favorites":
		if a.favoriteStore == nil {
			return BusinessConfig{}, errors.New("favorite store not initialized")
		}
		return configEnvelope(a.favoriteStore.Load())
	case "settings":
		settings, err := a.LoadSettings()
		if err != nil {
			return BusinessConfig{}, err
		}
		if a.aiConfigStore != nil {
			ai, err := a.LoadAIModels()
			if err != nil {
				return BusinessConfig{}, err
			}
			settings.AI.Models, settings.AI.MaxTurns = ai.Models, ai.MaxTurns
		}
		return configEnvelope(settings, nil)
	default:
		return BusinessConfig{}, fmt.Errorf("unknown business config %q", kind)
	}
}

func decodeConfig[T any](raw string, save func(T) error) error {
	var data T
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return err
	}
	return save(data)
}

func (a *App) SaveBusinessConfig(kind, expected, raw string) (BusinessConfig, error) {
	a.businessMu.Lock()
	defer a.businessMu.Unlock()
	if a.businessRestartRequired {
		return BusinessConfig{}, fmt.Errorf("data directory changed; restart before saving")
	}
	current, err := a.loadBusinessConfig(kind)
	if err != nil {
		return BusinessConfig{}, err
	}
	if expected == "" || current.Revision != expected {
		return BusinessConfig{}, store.ErrConflict
	}
	switch kind {
	case "sync":
		var input struct {
			Config cloudsync.SyncConfig `json:"config"`
			Token  string               `json:"token"`
		}
		if err = json.Unmarshal([]byte(raw), &input); err == nil {
			err = a.saveSyncConfig(input.Config, input.Token)
		}
	case "connections":
		err = decodeConfig[session.ConnectionStoreData](raw, a.saveConnections)
	case "identities":
		err = decodeConfig[session.IdentityStoreData](raw, a.saveIdentities)
	case "proxies":
		err = decodeConfig[session.ProxyStoreData](raw, a.saveProxies)
	case "tunnels":
		err = decodeConfig[session.TunnelStoreData](raw, a.saveTunnels)
	case "localState":
		err = decodeConfig[store.LocalState](raw, a.saveLocalState)
	case "ai":
		err = decodeConfig[store.AIStoreData](raw, a.saveAIModels)
	case "quickCommands":
		err = decodeConfig[store.QuickCommandData](raw, a.saveQuickCommands)
	case "favorites":
		err = decodeConfig[[]string](raw, a.saveFavoriteConnections)
	case "settings":
		err = decodeConfig[store.AppSettings](raw, func(settings store.AppSettings) error {
			if err := a.saveSettings(settings); err != nil {
				return err
			}
			if a.aiConfigStore != nil {
				if err := a.saveAIModels(store.AIStoreData{Models: settings.AI.Models, MaxTurns: settings.AI.MaxTurns}); err != nil {
					rollback := decodeConfig[store.AppSettings](current.JSON, a.saveSettings)
					return errors.Join(err, rollback)
				}
			}
			return nil
		})
	}
	if err != nil {
		return BusinessConfig{}, err
	}
	return a.loadBusinessConfig(kind)
}
