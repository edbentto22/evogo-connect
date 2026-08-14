package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/edbentto22/evogo-connect/internal/evogo"
)

var (
	flagConversationTenant string
	flagConversationJID    string
)

var startConversationCmd = &cobra.Command{
	Use:   "start-conversation",
	Short: "Cria uma conversa no Chatwoot para um contato já mapeado",
	Long: `Cria uma conversa aberta na inbox do tenant para um contato WhatsApp
adicionado com connect add-contact. O comando usa a credencial já cifrada no
connector; não é necessário informar nem expor o token do Chatwoot.

Exemplo:
  connect start-conversation --tenant demo --jid 5511999999999@s.whatsapp.net`,
	Args: cobra.NoArgs,
	RunE: runStartConversation,
}

func init() {
	startConversationCmd.Flags().StringVar(&flagConversationTenant, "tenant", "", "nome do tenant (obrigatório)")
	startConversationCmd.Flags().StringVar(&flagConversationJID, "jid", "", "JID do WhatsApp (ex: 5511999999999@s.whatsapp.net)")
	_ = startConversationCmd.MarkFlagRequired("tenant")
	_ = startConversationCmd.MarkFlagRequired("jid")
}

func runStartConversation(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, st, err := loadStoreCLI()
	if err != nil {
		return err
	}
	defer st.Close()

	tenant, err := st.GetTenantByName(ctx, flagConversationTenant)
	if err != nil {
		return fmt.Errorf("tenant '%s' não encontrado: %w", flagConversationTenant, err)
	}

	jid, _, err := evogo.ParseDirectJID(flagConversationJID)
	if err != nil {
		return fmt.Errorf("validate JID: %w", err)
	}

	contact, err := st.GetContactByJID(ctx, tenant.ID, jid)
	if err != nil {
		return fmt.Errorf("contact não mapeado; execute add-contact antes: %w", err)
	}
	if contact.SourceID != jid {
		return fmt.Errorf("contact mapeado com source_id diferente; execute add-contact novamente")
	}

	cw := chatwootClientFor(tenant)
	chatwootContact, err := cw.FindContactByIdentifier(ctx, jid)
	if err != nil {
		return fmt.Errorf("validate contact in Chatwoot: %w", err)
	}
	if chatwootContact.ID != contact.ChatwootContactID {
		return fmt.Errorf("contact mapping differs from Chatwoot; execute add-contact novamente")
	}
	if _, err := cw.EnsureContactInbox(ctx, chatwootContact, tenant.ChatwootInboxID, jid); err != nil {
		return fmt.Errorf("validate contact inbox: %w", err)
	}

	conversation, err := cw.CreateConversation(ctx, jid, contact.ChatwootContactID, tenant.ChatwootInboxID)
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}

	fmt.Printf("✓ Conversa aberta no Chatwoot: id=%d, inbox=%d\n", conversation.ID, conversation.InboxID)
	return nil
}
