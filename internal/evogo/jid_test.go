package evogo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDirectJID(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantJID    string
		wantNumber string
		wantErr    bool
	}{
		{name: "bare number", input: "5511999999999", wantJID: "5511999999999@s.whatsapp.net", wantNumber: "5511999999999"},
		{name: "direct jid", input: "5511999999999@s.whatsapp.net", wantJID: "5511999999999@s.whatsapp.net", wantNumber: "5511999999999"},
		{name: "group", input: "120363000@g.us", wantErr: true},
		{name: "letters", input: "abc@s.whatsapp.net", wantErr: true},
		{name: "empty number", input: "@s.whatsapp.net", wantErr: true},
		{name: "multiple separators", input: "5511@test@s.whatsapp.net", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jid, number, err := ParseDirectJID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantJID, jid)
			assert.Equal(t, tt.wantNumber, number)
		})
	}
}
