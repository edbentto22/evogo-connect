package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/edbentto22/evogo-connect/internal/chatwoot"
	"github.com/edbentto22/evogo-connect/internal/evogo"
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

	// Normaliza e valida JID individual (grupos não fazem parte desta etapa).
	jid, _, err := evogo.ParseDirectJID(flagJID)
	if err != nil {
		return fmt.Errorf("validate JID: %w", err)
	}

	// Busca ou cria o contato pelo identifier estável.
	cw := chatwootClientFor(tenant)
	contact, err := cw.FindContactByIdentifier(ctx, jid)
	if err != nil {
		if !errors.Is(err, chatwoot.ErrNotFound) {
			return fmt.Errorf("find contact: %w", err)
		}
		contact, err = cw.CreateContact(ctx, flagName, jid)
		if err != nil {
			createErr := err
			// Trata uma criação concorrente buscando novamente pelo identifier.
			contact, err = cw.FindContactByIdentifier(ctx, jid)
			if err != nil {
				return errors.Join(fmt.Errorf("create contact: %w", createErr), fmt.Errorf("refetch contact: %w", err))
			}
		}
		fmt.Printf("✓ Contato criado no Chatwoot: id=%d\n", contact.ID)
	} else {
		fmt.Printf("• Contato já existia (id=%d)\n", contact.ID)
	}

	contactInbox, err := cw.EnsureContactInbox(ctx, contact, tenant.ChatwootInboxID, jid)
	if err != nil {
		return fmt.Errorf("ensure contact inbox: %w", err)
	}
	if contactInbox.SourceID != jid {
		return fmt.Errorf("ensure contact inbox: source_id mismatch")
	}
	fmt.Println("✓ Vínculo com a inbox confirmado")

	// Persiste mapeamento
	cm := &store.ContactMap{
		TenantID:          tenant.ID,
		JID:               jid,
		ChatwootContactID: contact.ID,
		SourceID:          jid,
		DisplayName:       flagName,
	}
	if err := st.CreateContact(ctx, cm); err != nil {
		return fmt.Errorf("persist contact map: %w", err)
	}
	fmt.Printf("✓ Mapeamento salvo para chatwoot_contact_id=%d\n", contact.ID)
	return nil
}
