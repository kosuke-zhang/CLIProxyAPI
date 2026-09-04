package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestManagerMarkResultCodexNotFoundUsesShortCooldown(t *testing.T) {
	previous := quotaCooldownDisabled.Load()
	quotaCooldownDisabled.Store(false)
	t.Cleanup(func() { quotaCooldownDisabled.Store(previous) })

	for _, model := range []string{"gpt-5.6-sol", ""} {
		model := model
		t.Run(map[bool]string{true: "model", false: "auth"}[model != ""], func(t *testing.T) {
			manager := NewManager(nil, nil, nil)
			auth := &Auth{ID: "codex-404", Provider: "codex"}
			if _, err := manager.Register(context.Background(), auth); err != nil {
				t.Fatalf("register auth: %v", err)
			}

			started := time.Now()
			manager.MarkResult(context.Background(), Result{
				AuthID:   auth.ID,
				Provider: auth.Provider,
				Model:    model,
				Success:  false,
				Error:    &Error{HTTPStatus: http.StatusNotFound, Message: "not found"},
			})

			updated, ok := manager.GetByID(auth.ID)
			if !ok || updated == nil {
				t.Fatal("updated auth not found")
			}
			nextRetry := updated.NextRetryAfter
			if model != "" {
				state := updated.ModelStates[model]
				if state == nil {
					t.Fatalf("model state %q not found", model)
				}
				nextRetry = state.NextRetryAfter
			}
			assertCooldownNear(t, nextRetry, started.Add(codexNotFoundCooldown))
		})
	}
}

func TestManagerMarkResultNonCodexNotFoundKeepsLongCooldown(t *testing.T) {
	previous := quotaCooldownDisabled.Load()
	quotaCooldownDisabled.Store(false)
	t.Cleanup(func() { quotaCooldownDisabled.Store(previous) })

	manager := NewManager(nil, nil, nil)
	auth := &Auth{ID: "claude-404", Provider: "claude"}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	started := time.Now()
	manager.MarkResult(context.Background(), Result{
		AuthID:   auth.ID,
		Provider: auth.Provider,
		Model:    "claude-sonnet",
		Success:  false,
		Error:    &Error{HTTPStatus: http.StatusNotFound, Message: "not found"},
	})

	updated, ok := manager.GetByID(auth.ID)
	if !ok || updated == nil || updated.ModelStates["claude-sonnet"] == nil {
		t.Fatal("updated model state not found")
	}
	assertCooldownNear(t, updated.ModelStates["claude-sonnet"].NextRetryAfter, started.Add(12*time.Hour))
}

func assertCooldownNear(t *testing.T, got, want time.Time) {
	t.Helper()
	if delta := got.Sub(want); delta < -time.Second || delta > time.Second {
		t.Fatalf("cooldown deadline = %v, want near %v", got, want)
	}
}
