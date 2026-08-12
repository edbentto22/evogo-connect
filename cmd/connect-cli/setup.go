package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/edbentto22/evogo-connect/internal/chatwoot"
	"github.com/edbentto22/evogo-connect/internal/evogo"
	"github.com/edbentto22/evogo-connect/internal/store"
)

var (
	flagTenantName    string
	flagChatwootURL   string
	flagChatwootToken string
	flagChatwootAcct  int
	flagEvoURL        string
	flagEvoKey        string
	flagConnectURL    string
	flagEvoInstance   string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Cria inbox API no Chatwoot e registra tenant no connector",
	Long: `Cria uma inbox API Channel no Chatwoot apontando o webhook para este
connector, e persiste o tenant (com tokens cifrados) no Postgres do connector.

Exemplo:
  connect setup --name demo \
    --chatwoot-url https://cw.example.com \
    --chatwoot-token $CW_TOKEN \
    --chatwoot-account 1 \
    --evo-url http://localhost:8080 \
    --evo-key $EVO_GLOBAL_KEY \
    --evo-instance demo \
    --connect-url https://evogo-connect.example.com`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().StringVar(&flagTenantName, "name", "", "slug do tenant (obrigatório, único)")
	setupCmd.Flags().StringVar(&flagChatwootURL, "chatwoot-url", "", "URL base do Chatwoot (obrigatório)")
	setupCmd.Flags().StringVar(&flagChatwootToken, "chatwoot-token", "", "api_access_token do Chatwoot (obrigatório)")
	setupCmd.Flags().IntVar(&flagChatwootAcct, "chatwoot-account", 0, "ID da account no Chatwoot (obrigatório)")
	setupCmd.Flags().StringVar(&flagEvoURL, "evo-url", "", "URL base do evolution-go (obrigatório)")
	setupCmd.Flags().StringVar(&flagEvoKey, "evo-key", "", "GLOBAL_API_KEY do evolution-go (obrigatório)")
	setupCmd.Flags().StringVar(&flagEvoInstance, "evo-instance", "", "nome da instância no evolution-go (obrigatório)")
	setupCmd.Flags().StringVar(&flagConnectURL, "connect-url", "", "URL pública deste connector (obrigatório, usada como webhook)")
	_ = setupCmd.MarkFlagRequired("name")
	_ = setupCmd.MarkFlagRequired("chatwoot-url")
	_ = setupCmd.MarkFlagRequired("chatwoot-token")
	_ = setupCmd.MarkFlagRequired("chatwoot-account")
	_ = setupCmd.MarkFlagRequired("evo-url")
	_ = setupCmd.MarkFlagRequired("evo-key")
	_ = setupCmd.MarkFlagRequired("evo-instance")
	_ = setupCmd.MarkFlagRequired("connect-url")
}

func runSetup(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Carrega config e store
	_, st, err := loadStoreCLI()
	if err != nil {
		return err
	}
	defer st.Close()

	// 1. Cria inbox no Chatwoot
	cw := chatwoot.NewClient(flagChatwootURL, flagChatwootAcct, flagChatwootToken)
	webhookURL := strings.TrimRight(flagConnectURL, "/") + "/webhook/chatwoot"
	inbox, err := cw.CreateAPIInbox(ctx, "evogo-connect/"+flagTenantName, webhookURL)
	if err != nil {
		return fmt.Errorf("create inbox: %w", err)
	}
	fmt.Printf("✓ Inbox criada no Chatwoot: id=%d\n", inbox.ID)

	hmacToken := inbox.Channel.HMACToken
	if hmacToken == "" {
		fmt.Println("⚠ Chatwoot não retornou hmac_token (inbox sem HMAC — bridge vai aceitar webhooks sem assinatura)")
	}

	// 2. Configura webhook no evolution-go
	evo := evogo.NewClient(flagEvoURL, flagEvoKey)
	webhookURLForEvo := strings.TrimRight(flagConnectURL, "/") + "/webhook/evo/" + flagEvoInstance
	if err := evo.SetWebhook(ctx, flagEvoInstance, webhookURLForEvo, []string{
		"MESSAGES_UPSERT",
		"MESSAGES_UPDATE",
		"CONNECTION_UPDATE",
	}, false); err != nil {
		fmt.Printf("⚠ Falha ao configurar webhook no evolution-go: %v\n", err)
		fmt.Println("  Você pode configurar manualmente depois via POST /webhook/set/<instance>")
	} else {
		fmt.Printf("✓ Webhook configurado no evolution-go: %s\n", webhookURLForEvo)
	}

	// 3. Persiste tenant
	t := &store.Tenant{
		Name:              flagTenantName,
		ChatwootAccountID: flagChatwootAcct,
		ChatwootInboxID:   inbox.ID,
		ChatwootBaseURL:   strings.TrimRight(flagChatwootURL, "/"),
		ChatwootToken:     flagChatwootToken,
		ChatwootHMAC:      hmacToken,
		EvoInstanceName:   flagEvoInstance,
		EvoBaseURL:        strings.TrimRight(flagEvoURL, "/"),
		EvoAPIKey:         flagEvoKey,
	}
	if err := st.CreateTenant(ctx, t); err != nil {
		return fmt.Errorf("persist tenant: %w", err)
	}
	fmt.Printf("✓ Tenant '%s' registrado (id=%s)\n", t.Name, t.ID)
	fmt.Println()
	fmt.Println("Próximo passo — adicione um contato:")
	fmt.Printf("  connect add-contact --tenant %s --jid 5511999999999@s.whatsapp.net --name \"João\"\n", flagTenantName)
	return nil
}
