// Package log configura slog com PII masking e formato configurável.
//
// Política de PII:
//   - Telefone: mascara com `55****9999` (mantém DDI + últimos 4)
//   - pushName: trunca em 32 chars + hash curto
//   - content de mensagem: NUNCA logado em info; apenas SHA256 do conteúdo
//     para correlação. Em error, só se explicitamente necessário.
package log

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
)

// Init configura o logger global. Formato "json" ou "text".
func Init(level, format string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:       lvl,
		AddSource:   false,
		ReplaceAttr: nil,
	}

	var h slog.Handler
	if strings.ToLower(format) == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(h))
}

// MaskPhone mascara um telefone mantendo DDI e últimos 4 dígitos.
// "5511999999999" → "55****9999"
func MaskPhone(phone string) string {
	if phone == "" {
		return ""
	}
	// strip @s.whatsapp.net suffix
	if i := strings.Index(phone, "@"); i > 0 {
		phone = phone[:i]
	}
	// strip + if present
	phone = strings.TrimPrefix(phone, "+")
	if len(phone) < 6 {
		return "****"
	}
	ddi := phone[:2]
	last := phone[len(phone)-4:]
	return ddi + "****" + last
}

// TruncName trunca um nome a 32 chars com hash identificador para desambiguação.
func TruncName(name string) string {
	if name == "" {
		return ""
	}
	if len(name) <= 32 {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	short := hex.EncodeToString(sum[:])[:6]
	return name[:26] + "..." + short
}

// ContentHash retorna SHA256 hex do conteúdo de mensagem — seguro pra logar
// em info-level como chave de correlação.
func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// FromCtx devolve o logger do contexto ou o default.
func FromCtx(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// WithCtx anexa um logger ao contexto.
func WithCtx(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

type ctxKey struct{}
