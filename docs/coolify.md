# Deploy de produção no Coolify

Este é o procedimento de produção para o `evogo-connect`. Ele usa o proxy e o
TLS do próprio Coolify, mantém o PostgreSQL privado e executa as migrations na
inicialização do conector. O compose local em `deploy/docker-compose.yml`
continua sendo apenas uma referência de desenvolvimento.

> Requisito mínimo: Coolify `v4.0.0-beta.411`, que introduziu magic environment
> variables para fontes Git. O pacote precisa ser validado na versão instalada;
> o sufixo “ou mais recente” não é uma garantia irrestrita de compatibilidade.

## 1. Criar o recurso

1. No Coolify, crie um recurso **Docker Compose** a partir deste repositório Git.
2. Informe `deploy/docker-compose.coolify.yml` como caminho do compose.
3. Não adicione Chatwoot, Evolution Go, Caddy, Traefik nem redes customizadas ao
   stack. O Coolify cria a rede isolada e conecta seu próprio proxy.
4. Nas configurações de build, desative **Inject Build Args to Dockerfile**.
5. Na tela de variáveis, deixe senha, chave, token administrativo e credenciais
   temporárias marcados apenas como **Runtime Variable**; desmarque **Build
   Variable** para todos eles.
6. Confirme `BRIDGE_PAUSED=true` e faça o primeiro deploy.

Esse compose é específico para a execução do Coolify, que usa a raiz clonada
como `--project-directory`. Para validá-lo fora da plataforma, execute
`make validate-coolify`; não use diretamente `docker compose -f` sem informar a
raiz do projeto.

O Coolify gera e preserva automaticamente:

| Magic variable | Uso |
|---|---|
| `SERVICE_PASSWORD_64_POSTGRES` | senha interna do PostgreSQL e DSN do conector |
| `SERVICE_REALBASE64_32_CONNECTOR` | `CONNECT_MASTER_KEY`, chave AES-256 dos tokens persistidos |
| `SERVICE_HEX_64_ADMIN` | `ADMIN_TOKEN` dos endpoints administrativos |

Se estiver atualizando um recurso que já possua um valor **não vazio** em
`SERVICE_HEX_64_CONNECTOR_ADMIN`, copie esse mesmo valor para
`SERVICE_HEX_64_ADMIN` antes do redeploy e remova a variável antiga somente
depois de validar os endpoints administrativos. No primeiro deploy que falhou
com `ADMIN_TOKEN not set`, não há token anterior válido a preservar.

Não substitua nem regenere esses valores em redeploys. Guarde uma cópia segura
dos três para disaster recovery. A `SERVICE_REALBASE64_32_CONNECTOR` é
indispensável: sem a chave original, os tokens cifrados na tabela `tenants`
não podem ser recuperados mesmo que o backup do banco esteja íntegro.

O Coolify costuma habilitar variáveis de build e runtime por padrão. A etapa 4
é obrigatória: segredos do conector não podem ser enviados como build args nem
registrados no metadata/cache da imagem.

## 2. Domínio e TLS

No serviço `connector`, abra a configuração de domínios e associe:

```text
https://connect.exemplo.com:9090
```

A porta `9090` depois do domínio seleciona a porta **interna** do container; o
proxy do Coolify continua publicando HTTPS nas portas normais. Aponte o DNS do
host para a VPS e aguarde a emissão do certificado. Não associe domínio ao
serviço `postgres` e não publique a porta `5432`.

Valide pelo domínio público:

```bash
curl -fsS https://connect.exemplo.com/healthz
curl -fsS https://connect.exemplo.com/readyz
```

`/healthz` confirma que o processo responde. `/readyz` só retorna 200 quando o
banco também está acessível. O binário aplica migrations aditivas antes de
começar a servir tráfego.

## 3. Provisionar o tenant

Com os dois serviços saudáveis, cadastre temporariamente `CHATWOOT_TOKEN` e
`EVO_INSTANCE_TOKEN` como variáveis **Secret** do serviço `connector` e faça um
redeploy. As duas variáveis opcionais já estão declaradas no compose, portanto
aparecem na interface do Coolify e são injetadas apenas em runtime. Abra então
o **Terminal** desse serviço no Coolify e execute a CLI incluída na imagem:

```bash
/app/connect setup \
  --name demo \
  --chatwoot-url https://chatwoot.exemplo.com \
  --chatwoot-account 1 \
  --evo-url https://evolution.exemplo.com \
  --evo-instance demo \
  --connect-url https://connect.exemplo.com
```

Não digite os valores diretamente no comando, logs ou tickets. A CLI lê essas
duas credenciais do ambiente para que elas não apareçam no `argv` do processo.
Depois que o setup as cifrar no banco, remova os valores temporários do Coolify
e redeploye. O `DATABASE_URL` e a chave mestra permanecem disponíveis no
container pelas magic variables do compose.

Ainda no terminal, adicione os contatos necessários durante a Etapa 1:

```bash
/app/connect add-contact \
  --tenant demo \
  --jid 5511999999999@s.whatsapp.net \
  --name "João da Silva"

/app/connect list
/app/connect status
```

Para criar uma conversa diretamente na inbox e no contato já configurados, sem
expor o token do Chatwoot no terminal, use:

```bash
/app/connect start-conversation \
  --tenant demo \
  --jid 5511999999999@s.whatsapp.net
```

O comando confere o vínculo atual do contato antes de abrir a conversa. Depois,
envie a resposta nessa conversa recém-aberta; ele não corrige mensagens que já
tenham sido enviadas em uma conversa antiga com vínculo incorreto.

Se o provisionamento externo falhar, corrija a credencial ou conectividade e
repita `setup` com o mesmo `--name`. Não apague o volume para tentar corrigir
uma falha de Chatwoot ou Evolution Go.

## 4. Gate de liberação

Um stack `healthy` comprova somente processo, configuração básica, migrations e
conectividade com o PostgreSQL. Ele **não** homologa a integração externa.

Antes de liberar tráfego real:

1. Confirme `/healthz` e `/readyz` em HTTPS.
2. Altere `BRIDGE_PAUSED=false` no Coolify e faça redeploy imediatamente antes
   do smoke. Se o teste falhar, restaure `true` e redeploye antes de investigar.
3. Confirme no Chatwoot que a inbox aponta para
   `https://connect.exemplo.com/webhook/chatwoot` e exige HMAC.
4. Envie uma resposta real pela UI do Chatwoot.
5. Confirme que ela chegou ao WhatsApp pela instância Evolution Go 0.7.2.
6. Confirme a entrada de auditoria com `/app/connect status`.
7. Execute `scripts/smoke-e2e.sh` de uma estação autorizada, com as
   credenciais das instâncias homologadas.

7. Envie um texto do WhatsApp pareado para a instância. Ele deve aparecer como
   mensagem recebida na inbox correspondente do Chatwoot; os logs mostram
   `bridge: w2c delivered` sem exibir conteúdo ou telefone completo.

O `connect setup` configura automaticamente uma URL de webhook exclusiva por
instância na Evolution Go. O segredo fica cifrado no Postgres e não aparece em
`connect status` nem nos logs do conector. Não configure um webhook global ou
uma URL manual na Evolution Go. Para rotacionar o segredo, execute novamente
`connect setup` com o mesmo `--name` e `--rotate-evo-webhook-secret`; limite o
acesso aos logs do proxy/Coolify, pois o caminho completo da URL pode aparecer
nos registros.

Após atualizar para esta versão, execute `connect setup` novamente para cada
tenant existente (mesmo `--name` e credenciais temporárias). Isso grava o
segredo cifrado e registra o webhook Evolution Go; não recria a inbox.

## 5. Backup e restore

Configure backup diário, criptografia e retenção fora da VPS. Para um backup
manual via SSH, obtenha no Coolify o ID/nome do container `postgres` e execute
no host. O arquivo só recebe o nome definitivo depois de ser validado:

```bash
set -euo pipefail
umask 077
POSTGRES_CONTAINER="<container-postgres-do-recurso>"
BACKUP="evogo-connect-$(date +%F-%H%M).dump"
TEMP_BACKUP="$(mktemp "${BACKUP}.tmp.XXXXXX")"
trap 'rm -f "$TEMP_BACKUP"' EXIT

docker exec "$POSTGRES_CONTAINER" \
  pg_dump -U connect -d evogo_connect -Fc \
  > "$TEMP_BACKUP"
docker exec -i "$POSTGRES_CONTAINER" pg_restore --list < "$TEMP_BACKUP" >/dev/null
mv "$TEMP_BACKUP" "$BACKUP"
sha256sum "$BACKUP" > "${BACKUP}.sha256"
```

Transfira o dump e checksum para armazenamento externo cifrado e teste restores
periodicamente. Guarde em um cofre separado os valores persistentes das três
magic variables. Nunca coloque esses valores no `.dump` sem proteção adicional.

Para restaurar, primeiro configure no recurso de destino **a chave mestra e as
demais magic variables originais antes do primeiro start**. Pare explicitamente
o serviço `connector` no Coolify, faça backup do estado atual e use um banco
vazio ou uma janela de manutenção aprovada. Só então restaure o dump:

```bash
POSTGRES_CONTAINER="<container-postgres-do-recurso>"
docker exec -i "$POSTGRES_CONTAINER" \
  pg_restore -U connect -d evogo_connect --clean --if-exists --exit-on-error \
  < evogo-connect-AAAA-MM-DD-HHMM.dump
```

Depois de um restore sem erro, inicie o `connector`, confirme `/readyz`, execute
`/app/connect list` para provar que a chave decifra os tenants e finalize com um
smoke real. Se a chave original foi perdida, não faça restore integral em um
banco ativo: preserve o dump offline para auditoria, inicie um banco vazio e
reprovisione todos os tenants e tokens.

## 6. Upgrade e redeploy

1. Faça backup do PostgreSQL e das magic variables.
2. Confirme que o volume `postgres_data` está persistente.
3. Atualize a revisão Git e redeploye pelo Coolify.
4. Não apague o volume e não altere as magic variables.
5. Aguarde `postgres` e `connector` ficarem healthy.
6. Confirme `/readyz`, tenants e um smoke real.

As migrations são executadas automaticamente e devem permanecer aditivas. Se
o conector não ficar healthy, consulte os logs antes de qualquer rollback ou
mudança de volume.

Ao atualizar uma instalação anterior à homologação dos contratos atuais,
injete temporariamente as credenciais runtime como na seção 3 e execute
novamente `/app/connect setup` com o mesmo `--name`. O comando reutiliza a inbox
e atualiza em segurança o segredo HMAC e o token individual da instância
Evolution Go.

## 7. Troubleshooting

### Domínio retorna 404

- Confirme que o domínio foi associado ao serviço `connector`, não ao projeto
  ou ao `postgres`.
- Confirme o formato `https://host:9090`; sem a porta, o Coolify pode apontar
  para um destino interno incorreto.
- Confirme que o webhook cadastrado no Chatwoot termina em
  `/webhook/chatwoot`.

### Domínio retorna 502 ou serviço unhealthy

- Consulte primeiro os logs do `connector` e do `postgres`.
- Confirme que as três magic variables foram resolvidas e não estão vazias.
- Confirme que `postgres` está healthy; o conector espera essa condição.
- Erro de chave base64 indica `CONNECT_MASTER_KEY` inválida. Não gere uma nova
  chave se já houver tenants cifrados.

### Após alterar um segredo, o serviço não inicia

- `SERVICE_REALBASE64_32_CONNECTOR` não deve ser rotacionada depois que existem
  tenants. Restaure a chave original; `/readyz` sozinho não comprova decriptação,
  então confirme também `/app/connect list`.
- O PostgreSQL não muda a senha do usuário quando um volume inicializado recebe
  outro `POSTGRES_PASSWORD`. Restaure a senha anterior. Uma rotação planejada
  exige janela de manutenção: altere primeiro a senha do role `connect` dentro
  do banco e só então atualize a magic variable e redeploye.

### `/healthz` responde, mas `/readyz` retorna 503

O processo está vivo, mas não alcança o banco. Verifique o healthcheck do
PostgreSQL, o volume e a rede gerenciada do recurso no Coolify. Não adicione
rede customizada nem exponha `5432` como tentativa de correção.

### Redeploy perdeu tenants

Confirme que o recurso reutilizou o volume nomeado `postgres_data`. Se o volume
foi removido, restaure o dump com a chave mestra original. Um redeploy normal
não recria o volume nem as magic variables.

### Stack saudável, mas mensagem não chega

Isso é falha de integração, não de deploy. Valide token individual e estado da
instância Evolution Go, segredo/HMAC da inbox Chatwoot, relógio da VPS, audit
log e kill switch. Consulte também [operations.md](operations.md).
