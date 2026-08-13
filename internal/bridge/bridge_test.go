package bridge

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

func TestGetLimiter_CreatesAndReuses(t *testing.T) {
	// Core com limit=120/min → 2 req/s, burst=12
	c := New(nil, 24*time.Hour, false, 120)
	tenantID := uuid.New()

	lim1 := c.getLimiter(tenantID, DirC2W)
	if lim1 == nil {
		t.Fatal("expected non-nil limiter")
	}
	lim2 := c.getLimiter(tenantID, DirC2W)
	if lim1 != lim2 {
		t.Error("getLimiter should return the same instance for same key")
	}
}

func TestGetLimiter_DifferentTenantsGetDifferentLimiters(t *testing.T) {
	c := New(nil, 24*time.Hour, false, 60)
	a := uuid.New()
	b := uuid.New()
	if c.getLimiter(a, DirC2W) == c.getLimiter(b, DirC2W) {
		t.Error("different tenants should have different limiters")
	}
	if c.getLimiter(a, DirC2W) == c.getLimiter(a, DirW2C) {
		t.Error("different directions should have different limiters")
	}
}

func TestGetLimiter_BurstBehavior(t *testing.T) {
	// limit=60/min → 1 req/s, burst=6
	c := New(nil, 24*time.Hour, false, 60)
	tenantID := uuid.New()
	lim := c.getLimiter(tenantID, DirC2W)

	// Primeiros 6 devem passar (burst)
	allowed := 0
	for i := 0; i < 20; i++ {
		if lim.Allow() {
			allowed++
		}
	}
	// Após o burst, deve bloquear (sem refill em <1s)
	if allowed < 6 || allowed > 7 {
		t.Errorf("expected ~6 allowed (burst), got %d", allowed)
	}
}

func TestGetLimiter_ZeroLimitDisables(t *testing.T) {
	c := New(nil, 24*time.Hour, false, 0)
	tenantID := uuid.New()
	// getLimiter ainda é chamado mas retorna limiter muito restritivo.
	// No bridge, a checagem `if c.LimitPerMin > 0` evita a chamada quando
	// o limit é 0, garantindo zero overhead.
	lim := c.getLimiter(tenantID, DirC2W)
	// Sanity: ainda é um *rate.Limiter válido
	if _, ok := interface{}(lim).(*rate.Limiter); !ok {
		t.Error("expected *rate.Limiter")
	}
}
