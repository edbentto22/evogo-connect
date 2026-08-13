package main

import (
	"context"
	"errors"
	"fmt"
	"os"
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
  export CHATWOOT_TOKEN=<api_access_token>
  export EVO_INSTANCE_TOKEN=<token_da_instancia>
  connect setup --name demo \
    --chatwoot-url https://cw.example.com \
    --chatwoot-account 1 \
    --evo-url http://localhost:8080 \
    --evo-instance demo \
    --connect-url https://evogo-connect.example.com`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().StringVar(&flagTenantName, "name", "", "slug do tenant (obrigatório, único)")
	setupCmd.Flags().StringVar(&flagChatwootURL, "chatwoot-url", "", "URL base do Chatwoot (obrigatório)")
	setupCmd.Flags().StringVar(&flagChatwootToken, "chatwoot-token", "", "api_access_token do Chatwoot (ou env CHATWOOT_TOKEN)")
	setupCmd.Flags().IntVar(&flagChatwootAcct, "chatwoot-account", 0, "ID da account no Chatwoot (obrigatório)")
	setupCmd.Flags().StringVar(&flagEvoURL, "evo-url", "", "URL base do evolution-go (obrigatório)")
	setupCmd.Flags().StringVar(&flagEvoKey, "evo-key", "", "token individual da instância Evolution Go (ou env EVO_INSTANCE_TOKEN)")
	setupCmd.Flags().StringVar(&flagEvoInstance, "evo-instance", "", "nome da instância no evolution-go (obrigatório)")
	setupCmd.Flags().StringVar(&flagConnectURL, "connect-url", "", "URL pública deste connector (obrigatório, usada como webhook)")
	_ = setupCmd.MarkFlagRequired("name")
	_ = setupCmd.MarkFlagRequired("chatwoot-url")
	_ = setupCmd.MarkFlagRequired("chatwoot-account")
	_ = setupCmd.MarkFlagRequired("evo-url")
	_ = setupCmd.MarkFlagRequired("evo-instance")
	_ = setupCmd.MarkFlagRequired("connect-url")
}

func runSetup(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	chatwootToken, err := setupCredential(flagChatwootToken, "CHATWOOT_TOKEN")
	if err != nil {
		return err
	}
	evoInstanceToken, err := setupCredential(flagEvoKey, "EVO_INSTANCE_TOKEN")
	if err != nil {
		return err
	}

	// Carrega config e store
	_, st, err := loadStoreCLI()
	if err != nil {
		return err
	}
	defer st.Close()

	// 1. Valida a instância antes de criar recursos no Chatwoot. Rotas
	// autenticadas do Evolution Go usam o token individual da instância.
	evo := evogo.NewClient(flagEvoURL, evoInstanceToken)
	status, err := evo.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("validate Evolution Go instance token: %w", err)
	}
	if status.Message != "success" {
		return fmt.Errorf("Evolution Go returned an incomplete instance status")
	}
	fmt.Println("✓ Token individual da instância Evolution Go validado")

	// 2. Atualiza tenant legado in-place quando o nome já existe. Isso migra
	// hmac_token/GLOBAL_API_KEY antigos sem quebrar IDs e contatos locais.
	cw := chatwoot.NewClient(flagChatwootURL, flagChatwootAcct, chatwootToken)
	existing, err := st.GetTenantByName(ctx, flagTenantName)
	if err == nil {
		inbox, err := cw.GetInbox(ctx, existing.ChatwootInboxID)
		if err != nil {
			return fmt.Errorf("load existing Chatwoot inbox %d: %w", existing.ChatwootInboxID, err)
		}
		if inbox.Secret == "" {
			return fmt.Errorf("existing Chatwoot inbox %d has no webhook secret", existing.ChatwootInboxID)
		}
		existing.ChatwootAccountID = flagChatwootAcct
		existing.ChatwootBaseURL = strings.TrimRight(flagChatwootURL, "/")
		existing.ChatwootToken = chatwootToken
		existing.ChatwootHMAC = inbox.Secret
		existing.EvoInstanceName = flagEvoInstance
		existing.EvoBaseURL = strings.TrimRight(flagEvoURL, "/")
		existing.EvoAPIKey = evoInstanceToken
		if err := st.UpdateTenantIntegration(ctx, existing); err != nil {
			return fmt.Errorf("update existing tenant: %w", err)
		}
		fmt.Printf("✓ Tenant '%s' atualizado sem recriar a inbox (id=%s)\n", existing.Name, existing.ID)
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("check existing tenant: %w", err)
	}

	// 3. Cria inbox no Chatwoot para um tenant novo.
	webhookURL := strings.TrimRight(flagConnectURL, "/") + "/webhook/chatwoot"
	inbox, err := cw.CreateAPIInbox(ctx, "evogo-connect/"+flagTenantName, webhookURL)
	if err != nil {
		return fmt.Errorf("create inbox: %w", err)
	}
	fmt.Printf("✓ Inbox criada no Chatwoot: id=%d\n", inbox.ID)

	webhookSecret := inbox.Secret
	if webhookSecret == "" {
		return fmt.Errorf("Chatwoot inbox %d created without webhook secret; remove the orphan inbox before retrying", inbox.ID)
	}

	// 4. Persiste tenant. O webhook Evolution Go → Chatwoot não faz parte
	// desta etapa e não é configurado até existir um handler de entrada seguro.
	t := &store.Tenant{
		Name:              flagTenantName,
		ChatwootAccountID: flagChatwootAcct,
		ChatwootInboxID:   inbox.ID,
		ChatwootBaseURL:   strings.TrimRight(flagChatwootURL, "/"),
		ChatwootToken:     chatwootToken,
		ChatwootHMAC:      webhookSecret,
		EvoInstanceName:   flagEvoInstance,
		EvoBaseURL:        strings.TrimRight(flagEvoURL, "/"),
		EvoAPIKey:         evoInstanceToken,
	}
	if err := st.CreateTenant(ctx, t); err != nil {
		return fmt.Errorf("persist tenant after creating Chatwoot inbox %d; remove the orphan inbox before retrying: %w", inbox.ID, err)
	}
	fmt.Printf("✓ Tenant '%s' registrado (id=%s)\n", t.Name, t.ID)
	fmt.Println()
	fmt.Println("Próximo passo — adicione um contato:")
	fmt.Printf("  connect add-contact --tenant %s --jid 5511999999999@s.whatsapp.net --name \"João\"\n", flagTenantName)
	return nil
}

// setupCredential prioriza a flag para compatibilidade e usa env como caminho
// seguro, evitando expor segredos no argv do processo em produção.
func setupCredential(flagValue, envName string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if value := os.Getenv(envName); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("credential missing: use the corresponding flag or env %s", envName)
}
