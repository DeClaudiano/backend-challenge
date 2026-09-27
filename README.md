# Backend Challenge — Distributed Transaction Processing in Go

Implementação do desafio de processamento distribuído de apostas em Go, com foco em:

* integridade financeira;
* precisão monetária;
* idempotência persistente;
* concorrência entre processos e instâncias;
* Inbox/Outbox;
* processamento at-least-once;
* recuperação de falhas;
* autenticação e autorização via OAuth 2.0/OIDC;
* PostgreSQL como fonte de verdade financeira;
* AWS SQS executado localmente via LocalStack;
* composição e lifecycle com Uber Fx.

---

## 1. Stack

| Responsabilidade  | Tecnologia                                 |
| ----------------- | ------------------------------------------ |
| Linguagem         | Go 1.23                                    |
| Composição        | Uber Fx                                    |
| HTTP              | `net/http`                                 |
| Autenticação      | OAuth 2.0 / OIDC                           |
| IdP local         | Keycloak 25                                |
| Persistência      | PostgreSQL 16                              |
| Driver PostgreSQL | `pgx`                                      |
| Mensageria        | AWS SQS                                    |
| SQS local         | LocalStack                                 |
| Ambiente          | Docker Compose                             |
| Migrations        | SQL versionado                             |
| Testes            | `testing`, integração, E2E e `-race`       |
| Observabilidade   | JSON logs + métricas Prometheus-compatible |

---

## 2. Arquitetura

A solução é organizada em camadas, mantendo o domínio independente de HTTP, SQS, PostgreSQL e Uber Fx.

Estrutura principal:

```text
cmd/
├── api/
└── worker/

internal/
├── adapters/
│   ├── http/
│   ├── oidc/
│   ├── postgres/
│   └── sqs/
│
├── application/
│   ├── consumers/
│   ├── events/
│   ├── ports/
│   ├── publisher/
│   ├── references/
│   ├── services/
│   └── wagering/
│
├── bootstrap/
├── config/
├── domain/
│   ├── ledger/
│   ├── money/
│   ├── wagering/
│   └── wallet/
│
└── observability/

migrations/
scripts/
tests/
├── integration/
├── e2e/
└── recovery/
```

As decisões arquiteturais e as garantias de consistência estão documentadas em:

* [`ARCHITECTURE.md`](ARCHITECTURE.md)
* [`docs/TEST-MATRIX.md`](docs/TEST-MATRIX.md)

---

## 3. Pré-requisitos

Para executar a solução localmente:

* Go 1.23+
* Docker
* Docker Compose
* Git

Verifique:

```bash
go version
docker version
docker compose version
```

---

## 4. Configuração

Copie o arquivo de exemplo:

```bash
cp .env.example .env
```

O `.env.example` contém somente valores apropriados para o ambiente local.

Não devem ser utilizados segredos reais no repositório.

Principais configurações:

```text
DATABASE_URL
SQS_QUEUE_URL
SQS_ENDPOINT
SQS_REGION
SQS_VISIBILITY_TIMEOUT
SQS_WAIT_TIME_SECONDS

OUTBOX_BATCH_SIZE
OUTBOX_LEASE_DURATION
OUTBOX_POLL_INTERVAL

REFERENCE_BATCH_SIZE
REFERENCE_MAX_ATTEMPTS
REFERENCE_BASE_BACKOFF
REFERENCE_MAX_BACKOFF
REFERENCE_TTL
```

Os valores podem ser ajustados por environment ou pelo Docker Compose.

---

# 5. Subindo o ambiente

O Docker Compose provisiona:

* PostgreSQL;
* Keycloak;
* LocalStack;
* API;
* worker;
* aplicação das migrations.

Suba o ambiente com:

```bash
docker compose up --build
```

Ou em background:

```bash
docker compose up --build -d
```

Verifique os serviços:

```bash
docker compose ps
```

Para acompanhar os logs:

```bash
docker compose logs -f
```

Logs de um serviço específico:

```bash
docker compose logs -f api
docker compose logs -f worker
docker compose logs -f postgres
docker compose logs -f keycloak
docker compose logs -f localstack
```

Para encerrar:

```bash
docker compose down
```

Para remover também os volumes locais:

```bash
docker compose down -v
```

> A remoção dos volumes apaga os dados locais do PostgreSQL, Keycloak e demais serviços persistidos pelo Compose.

---

# 6. Migrations

As migrations são versionadas no diretório:

```text
migrations/
```

Cada migration possui arquivos de aplicação e reversão:

```text
*.up.sql
*.down.sql
```

As migrations atuais incluem:

```text
000001_bootstrap
000002_transaction_reversal_claim
000003_pending_reference_retry
```

O serviço de migration do Compose aplica as migrations antes da inicialização dos componentes que dependem do banco.

## Aplicação manual

Consulte:

```text
scripts/migrate.sh
```

para execução manual das migrations.

Exemplo:

```bash
./scripts/migrate.sh up
```

Para reversão:

```bash
./scripts/migrate.sh down
```

Caso o script utilize outro formato de argumentos, consulte o próprio script antes da execução.

As migrations são reversíveis e versionadas junto com o código.

---

# 7. Autenticação e autorização

Os endpoints de negócio são protegidos por OAuth 2.0 / OIDC.

O ambiente local utiliza Keycloak como IdP.

A aplicação valida o token recebido e utiliza a identidade autenticada para determinar o `providerId` autorizado.

O `providerId` enviado no payload não pode substituir a identidade determinada pelo token.

Quando houver divergência entre a identidade autenticada e o provider informado, a operação deve ser rejeitada.

A autenticação e autorização são implementadas fora do domínio e integradas através da camada de adapters.

## Identidades locais

O ambiente de desenvolvimento utiliza identidades de teste provisionadas pelo Keycloak/Compose.

Os nomes de realm, clients, usuários e roles devem ser consultados no arquivo de configuração do realm utilizado pelo ambiente:

```text
<arquivo-de-realm-do-projeto>
```

## Obtendo um token

Com o Keycloak em execução, obtenha um token utilizando `client_credentials` com o client provisionado para o teste.

Exemplo:

```bash
curl -X POST \
  http://localhost:<KEYCLOAK_PORT>/realms/<REALM>/protocol/openid-connect/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=client_credentials' \
  -d 'client_id=<CLIENT_ID>' \
  -d 'client_secret=<CLIENT_SECRET>'
```

O token retornado deve ser utilizado nos endpoints protegidos:

```bash
Authorization: Bearer <ACCESS_TOKEN>
```

> Utilize os valores reais definidos no realm local. Não coloque secrets reais no README ou no repositório.

---

# 8. API HTTP

## Health checks

Os health checks são públicos:

```http
GET /health/live
GET /health/ready
```

### Liveness

Indica que o processo está ativo.

### Readiness

Valida a disponibilidade das dependências necessárias para processamento, incluindo PostgreSQL e SQS.

---

## Endpoints protegidos

Os endpoints de negócio exigem Bearer Token OIDC.

```http
POST /wallets
GET /wallets/:walletId
GET /wallets/:walletId/ledger?cursor=...&limit=50
POST /wallets/:walletId/reconciliation

POST /wagering/transactions
GET /wagering/transactions/:transactionId
GET /providers/:providerId/wagering/transactions/:externalTransactionId
```

---

# 9. Abertura de carteira

Exemplo:

```http
POST /wallets
Content-Type: application/json
Authorization: Bearer <TOKEN>
```

```json
{
  "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
  "initialBalance": {
    "amount": "1000.00",
    "currency": "BRL"
  }
}
```

Uma abertura com saldo positivo cria:

* carteira;
* transação interna `OPENING`;
* lançamento de crédito no ledger;
* evento `WagerTransactionProcessed`;
* evento `WalletBalanceChanged`.

Essas alterações são confirmadas no mesmo commit SQL.

Saldo inicial zero não cria `OPENING`, ledger ou eventos financeiros.

Uma segunda abertura para o mesmo par:

```text
(playerId, currency)
```

deve resultar em conflito.

---

# 10. Operações de wagering

Exemplo:

```http
POST /wagering/transactions
Content-Type: application/json
Authorization: Bearer <TOKEN>
Idempotency-Key: provider-a:transaction-123
```

```json
{
  "providerId": "provider-a",
  "externalTransactionId": "transaction-123",
  "playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
  "walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
  "roundId": "round-987",
  "gameId": "fortune-chimp",
  "kind": "BET",
  "money": {
    "amount": "25.00",
    "currency": "BRL"
  }
}
```

Uma resposta após processamento pode ser:

```json
{
  "transactionId": "0192f298-345e-7e38-af88-e43f851a819d",
  "status": "PROCESSED",
  "balance": {
    "amount": "975.00",
    "currency": "BRL"
  },
  "idempotentReplay": false
}
```

O header:

```text
Idempotency-Key
```

é obrigatório.

A chave recebida pelo cliente não é substituída silenciosamente por uma chave calculada pelo servidor.

---

# 11. Idempotência

A idempotência é persistente e sobrevive ao reinício dos processos.

A aplicação utiliza:

* chave de idempotência;
* `(providerId, externalTransactionId)`;
* fingerprint determinístico do payload;
* constraints no PostgreSQL;
* Inbox para mensagens SQS.

O fingerprint não inclui:

* Idempotency-Key;
* metadados de transporte;
* informações não pertencentes ao conteúdo financeiro da operação.

Payload equivalente com a mesma chave:

```text
→ replay do resultado persistido
```

Mesma chave com conteúdo diferente:

```text
→ conflito
```

A mesma operação financeira não pode ser reaplicada usando uma segunda chave de idempotência.

Para operações já processadas, o replay retorna o resultado observado no processamento original, inclusive o saldo retornado naquele processamento.

---

# 12. Money

Os valores monetários não utilizam `float32` ou `float64`.

`Money` é um value object que encapsula:

* valor;
* moeda;
* operações aritméticas;
* comparação;
* serialização;
* validação.

As entradas externas utilizam strings decimais:

```json
{
  "amount": "25.00",
  "currency": "BRL"
}
```

A aplicação rejeita:

* valores vazios;
* valores negativos em entradas financeiras externas;
* `NaN`;
* `Infinity`;
* notação científica;
* escala superior à suportada;
* moedas incompatíveis;
* códigos de moeda não suportados.

As operações internas permitem valores negativos quando necessários para cálculos e diferenças, sem permitir saldo negativo da carteira.

A representação e os limites do tipo estão documentados em:

```text
ARCHITECTURE.md
```

---

# 13. Ledger

O ledger é append-only.

Cada lançamento contém:

* `id`;
* `walletId`;
* `transactionId`;
* direção;
* valor;
* saldo anterior;
* saldo posterior;
* timestamp.

A construção do lançamento valida:

```text
balanceAfter = balanceBefore ± money
```

O banco impõe a unicidade:

```text
(walletId, transactionId)
```

A edição e exclusão de lançamentos são bloqueadas por mecanismos de proteção no banco.

Correções financeiras são realizadas através de novos lançamentos, nunca alterando lançamentos existentes.

---

# 14. WagerTransaction

Os tipos externos suportados são:

```text
BET
WIN
LOSS
REFUND
ROLLBACK
```

Além disso:

```text
OPENING
```

é reservado para abertura interna de carteira.

Os estados incluem:

```text
PENDING
PENDING_REFERENCE
PROCESSED
REJECTED
FAILED
```

Uma transação terminal não sofre novas transições.

Reidratação de entidades não reaplica:

* movimentações;
* transições;
* eventos.

A máquina de estados e as regras de cada operação estão documentadas em:

```text
ARCHITECTURE.md
```

---

# 15. Referências pendentes

`REFUND` e `ROLLBACK` dependem de uma transação de referência.

Quando a referência ainda não existe, a operação é persistida como:

```text
PENDING_REFERENCE
```

Um worker independente executa tentativas posteriores com:

* backoff exponencial;
* número máximo de tentativas;
* TTL;
* persistência no PostgreSQL;
* retomada após reinicialização.

Quando a referência é localizada, a operação pode prosseguir.

Quando a referência não é encontrada após o limite configurado, a operação é finalizada como rejeitada com código de falha estável.

O worker não depende da memória de uma instância específica para continuar o processamento.

---

# 16. Concorrência

A coordenação financeira ocorre por carteira.

Não são utilizados locks globais para serializar toda a aplicação.

As garantias são sustentadas pelo PostgreSQL através de transações, locks e constraints.

O cenário obrigatório:

```text
Saldo inicial: 100.00 BRL

BET A: 80.00
BET B: 80.00

processamento simultâneo
```

deve produzir:

```text
1 operação PROCESSED
1 operação REJECTED por saldo insuficiente
saldo final: 20.00 BRL
1 débito no ledger
```

Reenvios não podem alterar o resultado.

Carteiras diferentes podem ser processadas em paralelo.

---

# 17. SQS

O ambiente local utiliza LocalStack para executar AWS SQS.

Filas provisionadas:

```text
wager-transactions.fifo
wager-transactions-dlq.fifo

wager-events.fifo
wager-events-dlq.fifo
```

A fila de entrada utiliza redrive para a DLQ.

O consumidor assume entrega at-least-once.

O `messageId` do envelope é utilizado como identidade durável da mensagem para o consumidor.

A Inbox mantém:

```text
consumerName
messageId
payload hash
receivedAt
completedAt
```

A Inbox possui unicidade para:

```text
(consumerName, messageId)
```

A mensagem somente é removida da fila após a conclusão durável do processamento.

Falhas transitórias permitem redelivery.

Falhas permanentes ou tentativas esgotadas seguem para a DLQ.

Os detalhes de:

* `MessageGroupId`;
* `MessageDeduplicationId`;
* visibility timeout;
* limite de redelivery;
* redrive;
* tratamento de mensagens inválidas;

estão documentados em `ARCHITECTURE.md`.

---

# 18. Transactional Outbox

As alterações financeiras e os eventos de integração são confirmados atomicamente no PostgreSQL.

A Outbox registra:

* `eventId`;
* `aggregateId`;
* `eventType`;
* `eventVersion`;
* payload;
* `occurredAt`;
* número de tentativas;
* `nextAttemptAt`;
* lease;
* publicação.

Um worker separado publica os eventos pendentes.

O publisher suporta:

* múltiplas instâncias;
* disputa por registros;
* leases;
* retry;
* backoff;
* recuperação de leases abandonados;
* republicação segura.

O `eventId` permanece o mesmo durante uma republicação.

O registro somente é marcado como publicado após a confirmação do broker.

Isso permite recuperação de falhas entre:

```text
commit → publicação
```

e:

```text
publicação → MarkPublished
```

O segundo cenário é coberto por teste específico de recovery.

---

# 19. Eventos

Os eventos de integração suportados são:

```text
WagerTransactionProcessed
WagerTransactionRejected
WalletBalanceChanged
WagerTransactionPendingReference
```

O envelope contém:

```text
eventId
eventType
aggregateId
correlationId
causationId
occurredAt
version
data
```

Os payloads utilizam valores monetários como strings decimais.

`WalletBalanceChanged` contém:

```text
walletId
transactionId
direction
money
balanceBefore
balanceAfter
walletVersion
```

O payload persistido na Outbox é tratado como snapshot imutável.

---

# 20. Observabilidade

A aplicação produz logs estruturados em JSON.

Os identificadores utilizados para rastreamento incluem, quando disponíveis:

```text
correlationId
messageId
transactionId
walletId
providerId
```

Credenciais, tokens e payloads financeiros completos não são registrados.

A API disponibiliza:

```http
GET /metrics
```

com métricas Prometheus-compatible.

As métricas incluem, entre outras:

* resultados de transações por status;
* replays idempotentes;
* retries;
* profundidade da DLQ;
* conflitos de concorrência;
* publicação da Outbox;
* falhas da Outbox;
* divergências de reconciliação;
* latência de processamento.

---

# 21. Reconciliação

A API fornece:

```http
POST /wallets/:walletId/reconciliation
```

A reconciliação:

1. lê o saldo armazenado;
2. reconstrói o saldo a partir do ledger;
3. calcula a diferença;
4. informa se os valores são consistentes;
5. registra divergências em logs e métricas.

A reconciliação não altera o saldo da carteira.

Exemplo:

```json
{
  "walletId": "...",
  "storedBalance": {
    "amount": "975.00",
    "currency": "BRL"
  },
  "calculatedBalance": {
    "amount": "975.00",
    "currency": "BRL"
  },
  "difference": {
    "amount": "0.00",
    "currency": "BRL"
  },
  "consistent": true,
  "checkedEntries": 2
}
```

---

# 22. Testes

## Testes gerais

```bash
go test ./...
```

## Race detector

```bash
go test -race ./...
```

## Static analysis

```bash
go vet ./...
```

## Formatação

Os arquivos Go devem ser formatados com:

```bash
gofmt -w <arquivos.go>
```

---

# 23. Testes de integração PostgreSQL

Os testes de integração utilizam PostgreSQL real.

Configure:

```bash
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable'
```

Execute:

```bash
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
  go test ./tests/integration -count=1 -v
```

A suíte cobre, entre outros:

* migrations;
* constraints;
* atomicidade;
* rollback;
* ledger append-only;
* concorrência financeira;
* duas apostas de `80.00` sobre saldo de `100.00`;
* múltiplas conexões/processos;
* 50 reenvios concorrentes;
* processamento paralelo de carteiras distintas;
* persistência da idempotência;
* invariantes financeiras.

Os testes utilizam um banco exclusivo para testes, pois os cenários realizam limpeza das tabelas financeiras.

Mais detalhes:

```text
tests/integration/README.md
```

---

# 24. Testes E2E

Os testes E2E utilizam infraestrutura real:

* PostgreSQL;
* Keycloak;
* LocalStack;
* API;
* worker.

Suba o ambiente:

```bash
docker compose up --build -d
```

Execute:

```bash
E2E_RUN=1 go test ./tests/e2e -count=1 -v
```

A suíte verifica fluxos autenticados e integração entre os componentes reais.

Entre os cenários estão:

* autenticação;
* autorização;
* isolamento entre providers;
* operações financeiras;
* idempotência;
* consultas;
* integração HTTP/SQS;
* eventos;
* infraestrutura real.

A suíte E2E permanece desabilitada por padrão para que:

```bash
go test ./...
```

não dependa automaticamente de infraestrutura externa.

Mais detalhes:

```text
tests/e2e/README.md
```

---

# 25. Testes de recovery

Os cenários de recuperação utilizam PostgreSQL e LocalStack reais.

Exemplo:

```bash
RECOVERY_RUN=1 \
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
go test ./tests -run 'TestSQS|TestOutbox' -count=1 -v
```

Os testes cobrem cenários como:

### Reentrega SQS

Interrupção depois do commit e antes da remoção da mensagem.

A mensagem deve ser entregue novamente sem gerar movimentação financeira duplicada.

### Outbox lease

Uma instância abandona um registro da Outbox.

Outra instância deve conseguir recuperar o trabalho após a expiração do lease.

### Publicação antes da confirmação

O broker confirma a publicação, mas o processo falha antes de `MarkPublished`.

A segunda execução deve republicar o evento utilizando o mesmo `eventId`.

Esse cenário demonstra a semântica at-least-once da Outbox.

### Pending reference

Uma reversão pode chegar antes da operação referenciada.

A operação é persistida como `PENDING_REFERENCE` e posteriormente retomada pelo worker de referências.

---

# 26. Múltiplas instâncias

A aplicação não depende de memória local para garantir:

* idempotência;
* consistência financeira;
* coordenação da Outbox;
* recuperação de trabalho.

Workers podem ser executados em múltiplas instâncias.

Exemplo, quando suportado pela configuração de portas do Compose:

```bash
docker compose up --build -d
docker compose up --scale worker=3 -d
```

A coordenação ocorre através do PostgreSQL e do broker.

Para a API, múltiplas réplicas devem ser executadas atrás de um mecanismo de distribuição de tráfego ou com configuração de portas adequada ao ambiente.

A aplicação não utiliza locks globais em memória para garantir as invariantes financeiras.

---

# 27. Testes obrigatórios do desafio

A implementação possui cobertura para os principais cenários exigidos:

| Cenário                             | Evidência                                             |
| ----------------------------------- | ----------------------------------------------------- |
| 50 reenvios da mesma aposta         | testes de integração                                  |
| Duas apostas de 80 sobre 100        | testes PostgreSQL                                     |
| Carteiras independentes em paralelo | testes PostgreSQL                                     |
| Múltiplas conexões/processos        | testes de concorrência                                |
| Idempotência persistente            | PostgreSQL                                            |
| Inbox                               | testes SQS/integração                                 |
| Redelivery                          | testes de recovery                                    |
| Outbox concorrente                  | testes de recovery                                    |
| Lease abandonado                    | testes de recovery                                    |
| Crash após publicação               | `TestOutboxRepublishesAfterPublishBeforeConfirmation` |
| Pending reference                   | testes de referência                                  |
| Reconciliação                       | testes de aplicação/E2E                               |
| Autenticação OIDC                   | testes E2E                                            |
| Isolamento de providers             | testes E2E                                            |
| PostgreSQL real                     | integração                                            |
| LocalStack real                     | E2E/recovery                                          |
| Keycloak real                       | E2E                                                   |
| Race detector                       | `go test -race ./...`                                 |

Os testes dependentes de infraestrutura devem ser executados explicitamente conforme os comandos desta documentação.

---

# 28. Parâmetros operacionais

Os principais parâmetros podem ser ajustados através de environment:

```text
SQS_VISIBILITY_TIMEOUT
SQS_WAIT_TIME_SECONDS
SQS_MAX_RECEIVE_COUNT

OUTBOX_BATCH_SIZE
OUTBOX_LEASE_DURATION
OUTBOX_POLL_INTERVAL

REFERENCE_BATCH_SIZE
REFERENCE_MAX_ATTEMPTS
REFERENCE_BASE_BACKOFF
REFERENCE_MAX_BACKOFF
REFERENCE_TTL
```

Os defaults locais estão disponíveis em:

```text
.env.example
```

Os parâmetros de retry e lease fazem parte das garantias de recuperação da aplicação.

---

# 29. Uber Fx e lifecycle

A aplicação utiliza Uber Fx para composição das dependências.

O bootstrap utiliza:

* `fx.Module`;
* `fx.Provide`;
* `fx.Invoke`;
* construtores para adapters e serviços;
* `fx.Lifecycle`.

O lifecycle controla:

* inicialização da API;
* workers;
* conexões;
* shutdown;
* cancelamento de processamento;
* encerramento das dependências.

Durante shutdown, novos trabalhos deixam de ser aceitos e os workers recebem contexto de cancelamento para finalizar ou liberar o trabalho em andamento.

As decisões de lifecycle estão documentadas em:

```text
ARCHITECTURE.md
```

---

# 30. Documentação adicional

| Documento                     | Conteúdo                           |
| ----------------------------- | ---------------------------------- |
| `ARCHITECTURE.md`             | decisões arquiteturais e garantias |
| `docs/REQUIREMENTS.md`        | rastreabilidade dos requisitos     |
| `docs/TEST-MATRIX.md`         | matriz de testes e evidências      |
| `tests/integration/README.md` | testes PostgreSQL                  |
| `tests/e2e/README.md`         | testes E2E                         |
| `AGENTS.md`                   | regras de desenvolvimento          |
| `CHALLENGER.MD`               | especificação do desafio           |

---

# 31. Limitações conhecidas

Os testes que dependem de infraestrutura externa exigem:

* Docker;
* PostgreSQL;
* Keycloak;
* LocalStack;
* dependências Go disponíveis.

Eles não devem ser considerados aprovados apenas pela execução de:

```bash
go test ./...
```

Os testes de integração, E2E e recovery possuem comandos próprios nesta documentação.

Qualquer cenário de infraestrutura que não tenha sido executado no ambiente de entrega deve ser tratado como não verificado, e não como aprovado implicitamente.

---

# 32. Verificação final

A verificação básica da solução pode ser executada com:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Para uma validação completa, executar também:

```bash
docker compose up --build -d
```

seguido das suítes:

```bash
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
  go test ./tests/integration -count=1 -v
```

```bash
E2E_RUN=1 go test ./tests/e2e -count=1 -v
```

```bash
RECOVERY_RUN=1 \
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
go test ./tests -run 'TestSQS|TestOutbox' -count=1 -v
```

Ao finalizar:

```bash
docker compose down
```

---

# 33. Reprodutibilidade

O projeto versiona:

* `go.mod`;
* `go.sum`;
* migrations;
* Docker Compose;
* configuração do ambiente local;
* configuração do IdP;
* código da aplicação;
* testes;
* documentação.

O objetivo é permitir que outra pessoa reproduza a solução a partir de um checkout limpo, utilizando apenas as dependências descritas neste README.

Não são armazenados no repositório:

* secrets reais;
* tokens;
* credenciais de produção;
* arquivos `.env` com informações sensíveis.

---

# 34. Status

A implementação contempla as principais garantias exigidas pelo desafio:

* precisão monetária sem ponto flutuante;
* invariantes financeiras no PostgreSQL;
* ledger append-only;
* idempotência persistente;
* Inbox;
* Transactional Outbox;
* processamento at-least-once;
* retries e backoff;
* recuperação de leases;
* referências pendentes;
* concorrência por carteira;
* autenticação OIDC;
* autorização por provider;
* SQS/LocalStack;
* PostgreSQL real;
* Keycloak;
* health checks;
* métricas;
* logs estruturados;
* composição e lifecycle com Uber Fx.

A aprovação final deve ser baseada na execução das suítes de teste correspondentes a cada requisito, conforme descrito em `docs/TEST-MATRIX.md`.
