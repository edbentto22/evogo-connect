package httpapi

import "encoding/json"

// extractInboxIDFromBody tenta extrair inbox_id de paths alternativos do payload.
// Alguns templates do Chatwoot colocam inbox_id dentro de conversation.inbox_id
// (e não no top-level).
func extractInboxIDFromBody(body []byte) int {
	var probe struct {
		Conversation struct {
			InboxID int `json:"inbox_id"`
		} `json:"conversation"`
		ContactInbox struct {
			InboxID int `json:"inbox_id"`
		} `json:"contact_inbox"`
	}
	_ = json.Unmarshal(body, &probe)
	if probe.Conversation.InboxID > 0 {
		return probe.Conversation.InboxID
	}
	if probe.ContactInbox.InboxID > 0 {
		return probe.ContactInbox.InboxID
	}
	return 0
}
