// connect: CLI para provisionar tenants e contatos.
//
// Subcomandos:
//
//	setup        — cria inbox API no Chatwoot, registra tenant no connector
//	add-contact  — cria contato no Chatwoot, mapeia JID → contact_id
//	list         — lista tenants
//	status       — status do connector (paused, tenants, etc)
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "connect",
	Short: "CLI do evogo-connect — provisiona tenants, contatos e opera o bridge",
	Long: `connect é a CLI para configurar e operar o evogo-connect.

Subcomandos principais:
  setup        Cria inbox API no Chatwoot e registra tenant.
  add-contact  Mapeia um JID WhatsApp a um contato no Chatwoot.
  list         Lista tenants configurados.
  status       Mostra status do connector (paused/running, contagens).
  pause        Ativa kill switch (pára de processar webhooks).
  resume       Desativa kill switch.`,
	SilenceUsage: true,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(setupCmd, addContactCmd, listCmd, statusCmd, pauseCmd, resumeCmd)
}
