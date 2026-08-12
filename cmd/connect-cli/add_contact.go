package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/edbentto22/evogo-connect/internal/store"
)

var (
	flagTenant string
	flagJID    string
	flagName   string
)

var addContactCmd = &cobra.Command{
	Use:   "add-contact",
	Short: "Cria contato no Chatwoot e mapeia JID WhatsApp → contact_id",
	Long: `Cria um contato no Chatwoot (API Channel inbox do tenant) usando
source_id = JID do WhatsApp, e persiste o mapeamento localmente.

Depois desse passo, quando o agente responder no Chatwoot para esse contato,
o connector entrega a mensagem no WhatsApp via evolution-go.

Exemplo:
  connect add-contact --tenant demo --jid 5511999999999@s.whatsapp.net --name "João"`,
	RunE: runAddContact,
}

func init() {
	addContactCmd.Flags().StringVar(&flagTenant, "tenant", "", "nome do tenant (obrigatório)")
	addContactCmd.Flags().StringVar(&flagJID, "jid", "", "JID do WhatsApp (ex: 5511999999999@s.whatsapp.net)")
	addContactCmd.Flags().StringVar(&flagName, "name", "", "nome do contato (obrigatório)")
	_ = addContactCmd.MarkFlagRequired("tenant")
	_ = addContactCmd.MarkFlagRequired("jid")
	_ = addContactCmd.MarkFlagRequired("name")
}

func runAddContact(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Carrega store
	_, st, err := loadStoreCLI()
	if err != nil {
		return err
	}
	defer st.Close()

	// Carrega tenant
	tenant, err := st.GetTenantByName(ctx, flagTenant)
	if err != nil {
		return fmt.Errorf("tenant '%s' não encontrado: %w", flagTenant, err)
	}

	// Normaliza JID
	jid := strings.TrimSpace(flagJID)
	if !strings.Contains(jid, "@") {
		jid = jid + "@s.whatsapp.net"
	}

	// Cria contato no Chatwoot
	cw := chatwootClientFor(tenant)
	contact, err := cw.CreateContact(ctx, tenant.ChatwootInboxID, flagName, jid)
	if err != nil {
		// Pode já existir (dedup por source_id)
		existing, err2 := cw.FindContactBySourceID(ctx, tenant.ChatwootInboxID, jid)
		if err2 != nil {
			return fmt.Errorf("create contact: %w", err)
		}
		contact = existing
		fmt.Printf("• Contato já existia (id=%d)\n", contact.ID)
	} else {
		fmt.Printf("✓ Contato criado no Chatwoot: id=%d\n", contact.ID)
	}

	// Persiste mapeamento
	cm := &store.ContactMap{
		TenantID:          tenant.ID,
		JID:               jid,
		ChatwootContactID: contact.ID,
		SourceID:          contact.SourceID,
		DisplayName:       flagName,
	}
	if err := st.CreateContact(ctx, cm); err != nil {
		return fmt.Errorf("persist contact map: %w", err)
	}
	fmt.Printf("✓ Mapeamento salvo: %s → chatwoot_contact_id=%d\n", jid, contact.ID)
	return nil
}
