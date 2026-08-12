package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func pauseHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Reason string `json:"reason"`
		}
		_ = c.ShouldBindJSON(&body)
		if err := d.Store.SetPaused(c.Request.Context(), true, body.Reason); err != nil {
			respondErr(c, http.StatusInternalServerError, "pause_failed", err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "paused", "reason": body.Reason})
	}
}

func resumeHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := d.Store.SetPaused(c.Request.Context(), false, ""); err != nil {
			respondErr(c, http.StatusInternalServerError, "resume_failed", err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "running"})
	}
}

func isPausedHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		paused, err := d.Store.IsPaused(c.Request.Context())
		if err != nil {
			respondErr(c, http.StatusInternalServerError, "check_failed", err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"paused": paused})
	}
}

func listTenantsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenants, err := d.Store.ListTenants(c.Request.Context())
		if err != nil {
			respondErr(c, http.StatusInternalServerError, "list_failed", err)
			return
		}
		// Não retorna tokens — só metadados
		out := make([]gin.H, 0, len(tenants))
		for _, t := range tenants {
			out = append(out, gin.H{
				"id":                  t.ID,
				"name":                t.Name,
				"chatwoot_account_id": t.ChatwootAccountID,
				"chatwoot_inbox_id":   t.ChatwootInboxID,
				"chatwoot_base_url":   t.ChatwootBaseURL,
				"has_chatwoot_hmac":   t.ChatwootHMAC != "",
				"evo_instance_name":   t.EvoInstanceName,
				"evo_base_url":        t.EvoBaseURL,
				"has_evo_secret":      t.EvoWebhookSecret != "",
				"created_at":          t.CreatedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"tenants": out})
	}
}
