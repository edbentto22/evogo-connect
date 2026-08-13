package evogo

import (
	"errors"
	"strings"
	"unicode"
)

// ParseDirectJID normaliza contatos individuais e rejeita grupos, domínios
// desconhecidos e números não numéricos.
func ParseDirectJID(value string) (jid, number string, err error) {
	jid = strings.TrimSpace(value)
	if !strings.Contains(jid, "@") {
		jid += "@s.whatsapp.net"
	}
	parts := strings.Split(jid, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "s.whatsapp.net" {
		return "", "", errors.New("evogo: invalid direct WhatsApp JID")
	}
	for _, r := range parts[0] {
		if !unicode.IsDigit(r) || r > unicode.MaxASCII {
			return "", "", errors.New("evogo: invalid direct WhatsApp JID")
		}
	}
	return jid, parts[0], nil
}
