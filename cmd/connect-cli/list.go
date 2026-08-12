package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	applog "github.com/edbentto22/evogo-connect/internal/log"
	"github.com/edbentto22/evogo-connect/internal/store"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista tenants configurados",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, st, err := loadStoreCLI()
	if err != nil {
		return err
	}
	defer st.Close()

	tenants, err := st.ListTenants(ctx)
	if err != nil {
		return err
	}
	if len(tenants) == 0 {
		fmt.Println("(nenhum tenant — rode `connect setup` para provisionar)")
		return nil
	}

	fmt.Printf("%-20s %-15s %-10s %-30s %-20s\n", "NAME", "INBOX", "ACCOUNT", "EVO INSTANCE", "CREATED")
	fmt.Println(strings.Repeat("-", 100))
	for _, t := range tenants {
		fmt.Printf("%-20s %-15d %-10d %-30s %-20s\n",
			t.Name, t.ChatwootInboxID, t.ChatwootAccountID,
			t.EvoInstanceName, t.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	return nil
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Status do connector (paused/running, contagens)",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, st, err := loadStoreCLI()
	if err != nil {
		return err
	}
	defer st.Close()

	paused, err := st.IsPaused(ctx)
	if err != nil {
		return err
	}
	tenants, _ := st.ListTenants(ctx)
	fmt.Println("evogo-connect status")
	fmt.Println("--------------------")
	if paused {
		fmt.Println("Estado:        PAUSADO (kill switch ativo)")
	} else {
		fmt.Println("Estado:        RODANDO")
	}
	fmt.Printf("Tenants:       %d\n", len(tenants))
	for _, t := range tenants {
		fmt.Printf("  • %s (inbox=%d, evo=%s)\n", t.Name, t.ChatwootInboxID, t.EvoInstanceName)
	}
	// Auditoria recente (últimas 5)
	var recent []store.BridgeLogEntry
	rows, err := st.Pool().Query(ctx, `
		SELECT tenant_id, direction, external_message_id, jid, status, error_code, latency_ms
		FROM bridge_log ORDER BY created_at DESC LIMIT 5
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var e store.BridgeLogEntry
			if err := rows.Scan(&e.TenantID, &e.Direction, &e.ExternalMessageID, &e.JID, &e.Status, &e.ErrorCode, &e.LatencyMS); err == nil {
				recent = append(recent, e)
			}
		}
	}
	if len(recent) > 0 {
		fmt.Println("\nÚltimas 5 entradas do audit log:")
		for _, e := range recent {
			jid := applog.MaskPhone(e.JID)
			fmt.Printf("  [%s] dir=%s msg=%s jid=%s status=%s err=%s latency=%dms\n",
				e.Direction, e.Direction, e.ExternalMessageID, jid, e.Status, e.ErrorCode, e.LatencyMS)
		}
	}
	return nil
}

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Ativa o kill switch (pára de processar webhooks)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, st, err := loadStoreCLI()
		if err != nil {
			return err
		}
		defer st.Close()
		reason, _ := cmd.Flags().GetString("reason")
		if err := st.SetPaused(ctx, true, reason); err != nil {
			return err
		}
		fmt.Println("✓ Bridge pausado")
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Desativa o kill switch",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, st, err := loadStoreCLI()
		if err != nil {
			return err
		}
		defer st.Close()
		if err := st.SetPaused(ctx, false, ""); err != nil {
			return err
		}
		fmt.Println("✓ Bridge rodando")
		return nil
	},
}

func init() {
	pauseCmd.Flags().String("reason", "", "motivo da pausa (auditado)")
}
