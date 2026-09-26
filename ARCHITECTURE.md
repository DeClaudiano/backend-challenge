# Arquitetura — Processamento Distribuído de Apostas

**Estado:** implementação principal concluída; principais suítes de concorrência, integração e E2E foram executadas com sucesso no ambiente local.

**Última revisão:** 2026-09-27

## 1. Objetivo e princípios

O serviço possui API HTTP e consumidor SQS com as mesmas regras financeiras. PostgreSQL participa da consistência: saldo, transação, ledger, Inbox e Outbox são alterados por transações SQL, constraints e locks. Clean Architecture, DDD, SOLID e inversão de dependências são obrigatórios; domínio e aplicação não dependem de HTTP, PostgreSQL, SQS, AWS SDK, Fx ou Keycloak.

A solução contém domínio, application services, repositories PostgreSQL, API HTTP protegida por OIDC, consumidor SQS, Inbox, Outbox, publisher, worker de referências pendentes, observabilidade e lifecycle Fx.

## 2. Estrutura de diretórios

```text
cmd/api/                         # entrypoint HTTP
cmd/worker/                      # entrypoint de workers

internal/

  domain/{money,wallet,wagering,ledger,events}/

  application/{ports,wallet,wagering,inbox,outbox,references}/

  adapters/{http,postgres,sqs,oidc,observability}

  bootstrap/                      # composição Fx e lifecycle
  config/                         # environment e validação

migrations/                       # migrations up/down
deploy/keycloak/                  # realm local
deploy/compose/                   # scripts de infraestrutura

tests/{integration,e2e,concurrency,recovery}/

Dockerfile
docker-compose.yml

go.mod
go.sum
```

Dependências: `domain` não conhece infraestrutura; `application` depende do domínio e portas; adaptadores implementam portas; `bootstrap` compõe tudo. Handlers e consumers traduzem transporte e chamam casos de uso, sem regras financeiras.

## 3. Bootstrap e ciclo de vida

* `go.mod` declara Go 1.23 e dependências do Fx, pgx/pgxpool e AWS SDK v2 SQS.
* `cmd/api` e `cmd/worker` inicializam a composição Fx.
* `internal/config` carrega e valida ambiente, HTTP, PostgreSQL, OIDC e parâmetros operacionais de SQS, Outbox e `PENDING_REFERENCE`.
* `internal/bootstrap` fornece logger JSON, configuração, pool PostgreSQL, cliente SQS, configuração OIDC, servidor HTTP e worker; lifecycle fecha pool e interrompe o worker de forma context-aware.
* `GET /health/live` responde pela vida do processo.
* `GET /health/ready` verifica PostgreSQL e SQS e retorna `503` quando uma dependência não está pronta.
* A API expõe os endpoints de negócio e aplica middleware OIDC antes das rotas privadas.
* O worker compõe consumidor SQS, publisher da Outbox e worker de referências pendentes no mesmo container Fx.

## 4. Ambiente local

Docker Compose provisiona PostgreSQL 16, Keycloak 25, LocalStack com SQS e os processos API/worker. O serviço `migrate` aguarda PostgreSQL e aplica a migration inicial antes dos processos Go. LocalStack cria as filas FIFO de transações e eventos, suas DLQs e redrive. Keycloak importa `deploy/keycloak/realm.json`, com clientes de serviço para desenvolvimento local; credenciais locais são exemplos, não segredos de produção.

O Dockerfile é multi-stage e produz executáveis separados; a imagem executa API por padrão e Compose sobrescreve o comando do worker. Environment é configurável por `.env.example` e pelas variáveis do Compose. Os parâmetros operacionais de retry, TTL, visibility timeout e `maxReceiveCount` continuam configuráveis; nenhum valor é tratado como requisito fixo do desafio.

## 5. PostgreSQL e migrations

A biblioteca escolhida é `pgx`/`pgxpool` com SQL explícito. As migrations versionadas criam e evoluem as tabelas `wallets`, `wager_transactions`, `wallet_ledger_entries`, `inbox_messages` e `outbox_events`, além das constraints e índices que fazem parte do modelo de consistência:

* carteira com saldo não negativo, versão mínima 1 e unicidade `(player_id,currency)`;
* transação com origem interna/externa, tipos e estados válidos, FKs e unicidades de provider/idempotency/external ID;
* ledger com amount positivo, equação de saldo, FK restritiva e unique `(wallet_id,transaction_id)`;
* Inbox com PK `(consumer_name,message_id)` e hash;
* Outbox com `event_id`, payload, tentativas, próximo envio, lease e publicação;
* índice único parcial para impedir duas reversões diretas processadas sobre a mesma referência;
* claim persistido `reversed_by_transaction_id`/`reversal_kind` para reidratar o estado de reversão do agregado;
* trigger que impede `UPDATE`/`DELETE` no ledger.

A fronteira de cada operação financeira usa uma única `pgx.Tx` compartilhada por repositórios via contexto: lock da carteira, mudança de saldo/versão, ledger e estado da transação, além de Inbox/Outbox, são confirmados juntos quando fazem parte do caso de uso. Nada é publicado antes do commit. Migrations são versionadas com arquivos `.up.sql` e `.down.sql`; a aplicação no Compose usa o serviço `migrate`.

A camada `application/services` coordena essa fronteira sem duplicar regras financeiras: `WalletService.Open` persiste carteira, `OPENING` e ledger atomicamente; `TransactionService.Process` resolve replay/conflito por fingerprint, bloqueia a carteira e referências quando aplicável, executa o caso de uso puro e persiste transação, saldo e ledger no mesmo `UnitOfWork`. `QueryService` fornece consultas de wallet, transaction, ledger paginado por cursor opaco e reconciliação.

## 6. Domínio e invariantes aprovados

### Money

`Money` é um value object imutável, armazenado como `int64` em unidades mínimas e código monetário ISO 4217; a implementação mantém uma allow-list de códigos suportados e rejeita códigos apenas sintaticamente válidos que não estejam nessa lista; parsing, aritmética e persistência nunca usarão `float32`/`float64`.

A entrada externa usa string decimal e o contrato mostra `25.00`, com escala fixa de duas casas; o texto de `challenge.md` não resolve literalmente se `25` é forma equivalente aceita. Se formas equivalentes forem aceitas, a normalização será documentada antes do hash; não haverá arredondamento silencioso.

Moedas incompatíveis, escala excedente, notação científica, valores vazios/negativos externos e overflow serão rejeitados.

### Wallet

É a raiz do agregado financeiro: identidade, jogador, moeda, saldo, versão e timestamps.

`(playerId,currency)` é único; saldo nunca fica negativo; a versão começa em 1 e só aumenta quando o saldo muda; moeda da movimentação deve coincidir; mudança de saldo e lançamento correspondente são atômicos.

Criação e reidratação são distintas; reidratação não reaplica efeitos.

### WagerTransaction

Suporta `OPENING`, `BET`, `WIN`, `LOSS`, `REFUND` e `ROLLBACK`, com estados `PENDING`, `PENDING_REFERENCE`, `PROCESSED`, `REJECTED` e `FAILED`; os três últimos são terminais.

Replays retornam resultado persistido sem reaplicar operação. `OPENING` é somente interno e não aceita metadados externos.

`BET` debita valor positivo com saldo suficiente; `WIN` credita valor positivo; `LOSS` exige `0.00`, não altera saldo/versão/ledger e emite somente Processed; `REFUND` credita integralmente uma `BET` processada; `ROLLBACK` desfaz integralmente uma `BET`, `WIN` ou `REFUND` processada.

Reversões exigem referência por `(providerId,referenceExternalTransactionId)`, contexto compatível e valor igual; reversões parciais não existem.

**Política adotada:** uma transação processada só pode ser consumida por uma reversão direta bem-sucedida, seja `REFUND` ou `ROLLBACK`. Assim a mesma aposta não pode receber duas devoluções financeiras. Um `ROLLBACK` de uma `REFUND` processada é permitido para desfazer o crédito da restituição, também uma única vez.

Essa é uma interpretação arquitetural necessária porque o desafio exige impedir devolução duplicada, mas não prescreve toda a matriz de combinações.

### Ledger, Inbox e Outbox

Ledger é append-only, imutável e registra carteira, transação, direção, valor, saldos anterior/posterior e timestamp.

`LOSS` e rejeições não geram lançamento.

Inbox é única por consumidor/mensagem e valida hash em redelivery. O consumidor SQS usa o port/repository da Inbox e a `pgx.Tx` reentrante do `UnitOfWork`: claim, operação financeira e conclusão são confirmados juntos; a mensagem só é removida depois do commit.

Outbox contém snapshot imutável com `eventId` estável, tipo/versão, agregado, payload, ocorrência, tentativas, próximo envio e lease.

## 7. Concorrência e idempotência

A estratégia implementada é lock pessimista por carteira (`SELECT ... FOR UPDATE`) dentro de transação `READ COMMITTED`, sem lock global.

Carteiras diferentes avançam em paralelo.

Os locks adicionais de referência seguem a ordem wallet -> referência dentro do processamento; constraints e checks do PostgreSQL permanecem autoridade mesmo com múltiplas instâncias.

Idempotência é persistente e compartilhada entre HTTP e SQS:

* unicidades por `(providerId,idempotencyKey)` e `(providerId,externalTransactionId)`;
* hash SHA-256 de JSON canônico dos campos de negócio;
* exclusão dos metadados de transporte;
* snapshot do resultado original.

Chave/conteúdo iguais retornam replay; chave com conteúdo diferente e operação externa reutilizada com outra chave geram conflito.

## 8. Referências, SQS e eventos

`REFUND`/`ROLLBACK` sem referência ficam `PENDING_REFERENCE`, com retries exponenciais e TTL/máximo de tentativas persistidos.

Após o limite, a operação é marcada como `REJECTED` com código estável.

Referência pendente aguarda; referência rejeitada/falha ou incompatível gera rejeição definitiva.

Entrada usa `wager-transactions.fifo` e DLQ.

`MessageGroupId` é derivado do `aggregateId`/wallet; `MessageDeduplicationId` usa o `eventId` nos eventos de Outbox.

FIFO é otimização, não substitui Inbox/idempotência no banco.

A mensagem só é removida após commit; falha transitória mantém redelivery; poison/permanente chega à DLQ.

Em SIGTERM, polling para e o trabalho em andamento conclui ou libera visibilidade.

Outbox é publicada por worker separado com múltiplos publishers, `FOR UPDATE SKIP LOCKED`, lease, backoff e recuperação.

Publicação é at-least-once; `eventId` permanece igual em republicação.

Eventos exigidos:

```text
WagerTransactionProcessed
WagerTransactionRejected
WalletBalanceChanged
WagerTransactionPendingReference
```

## 9. Autenticação e autorização

Keycloak é o IdP local escolhido, com OAuth 2.0/OIDC e `client_credentials`; não haverá emissão própria nem cadastro próprio de senhas.

Tokens validam issuer, audience, assinatura RSA e expiração.

Atualmente, `providerId` é derivado do claim padrão OIDC `sub`; não há claim customizado, role ou scope adicional para representá-lo.

O middleware coloca `sub` e a identidade autenticada no contexto, e nenhum valor vindo do body, query ou header pode sobrescrevê-lo.

Providers acessam somente suas próprias transações e replays.

Operações de carteira são internas e exigem o client OIDC existente `wallet-internal` identificado pelo claim `azp`; não foi criado RBAC adicional.

A arquitetura interpreta GET de carteira/ledger como operação de carteira e os restringirá ao serviço interno; o desafio não fornece matriz específica para esses GET, portanto essa interpretação permanece documentada como ambiguidade.

Health checks são públicos.

## 10. Testes

Há testes unitários de domínio/application/adapters e suítes de integração, E2E e recovery preparadas para PostgreSQL, Keycloak e LocalStack.

Os testes de integração PostgreSQL foram executados contra PostgreSQL real no ambiente local.

A suíte de integração completa executada foi:

```bash
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
go test ./tests/integration -count=1 -v
```

Resultado:

```text
TestProcessHelper                              PASS
TestPostgresAtomicityConstraintsAndPersistence PASS
TestPostgresDistributedWalletContention        PASS
TestPostgresIndependentWalletsProceedInParallel PASS
TestPostgresConcurrentIdempotency              PASS
```

A suíte de integração também foi executada com o race detector:

```bash
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
go test -race ./tests/integration -count=1 -v
```

Resultado:

```text
TestProcessHelper                              PASS
TestPostgresAtomicityConstraintsAndPersistence PASS
TestPostgresDistributedWalletContention        PASS
TestPostgresIndependentWalletsProceedInParallel PASS
TestPostgresConcurrentIdempotency              PASS
```

Os cenários de concorrência distribuída e idempotência concorrente também foram executados isoladamente:

```bash
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
go test ./tests/integration \
  -run 'TestPostgres(DistributedWalletContention|ConcurrentIdempotency)$' \
  -count=1 -v
```

Ambos os testes passaram.

Os testes E2E foram executados contra a infraestrutura Docker real:

```bash
E2E_RUN=1 go test ./tests/e2e -count=1 -v
```

Resultado:

```text
TestE2EHTTPAuthenticationIdempotencyAndIsolation PASS
TestE2ESQSHTTPReplayAndOutbox                    PASS
```

Esses testes validaram, entre outros aspectos:

* autenticação;
* autorização;
* isolamento entre providers;
* operações financeiras;
* idempotência;
* replay HTTP/SQS;
* integração com SQS;
* Outbox;
* eventos;
* infraestrutura real.

A execução dessas suítes demonstra as garantias verificadas no ambiente local. Testes de recovery específicos devem continuar sendo executados explicitamente quando a validação dessas simulações de falha fizer parte da entrega.

Os comandos gerais de verificação permanecem:

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
```

## 11. Ambiguidades preservadas

1. `challenge.md` não afirma literalmente se a string `25` deve ser aceita ou se somente `25.00` é válida, embora mostre `25.00`, exija escala fixa de duas casas e rejeite escala excedente.

2. “Operações de carteira” não é decomposto em autorização para GET de carteira/ledger; a arquitetura adotou acesso interno conservador.

3. O desafio exige coerência entre `REFUND` e `ROLLBACK` e ausência de devolução duplicada, mas não define toda a matriz; a política de uma reversão direta consumidora por transação é a decisão adotada.

4. TTL, visibility timeout, `maxReceiveCount`, backoff, jitter e demais parâmetros operacionais não têm valores normativos e permanecem configuráveis.

5. O checkout possui dois clients OIDC de provider (`wallet-api` e `wallet-provider-b`) para a matriz E2E de isolamento; a prova definitiva depende da execução da infraestrutura real.

## 12. Estado de verificação

A implementação foi validada no ambiente local com PostgreSQL real e infraestrutura Docker para os cenários executados.

Foram comprovados:

* suíte completa de integração PostgreSQL;
* atomicidade e persistência;
* concorrência distribuída entre processos;
* processamento paralelo de carteiras independentes;
* idempotência concorrente;
* execução com `go test -race` na suíte de integração;
* autenticação e autorização via E2E;
* isolamento entre providers;
* replay HTTP/SQS;
* integração com SQS;
* Transactional Outbox;
* execução contra PostgreSQL, Keycloak e LocalStack reais nos testes E2E.

A execução da suíte de integração produziu sucesso em todos os testes PostgreSQL previstos, incluindo:

```text
TestPostgresAtomicityConstraintsAndPersistence
TestPostgresDistributedWalletContention
TestPostgresIndependentWalletsProceedInParallel
TestPostgresConcurrentIdempotency
```

A execução com race detector também terminou sem detecção de data races.

A suíte E2E terminou com sucesso em:

```text
TestE2EHTTPAuthenticationIdempotencyAndIsolation
TestE2ESQSHTTPReplayAndOutbox
```

Os testes específicos de recovery que simulam interrupções entre etapas críticas devem ser executados explicitamente quando se desejar validar essas condições de falha no ambiente de entrega.

O comportamento de `PENDING_REFERENCE` é durável: pendências são retomadas pelo worker, usam backoff e TTL/máximo de tentativas persistidos, referências ainda pendentes continuam aguardando e referências terminadas como `REJECTED`/`FAILED` geram rejeição definitiva com `REFERENCE_NOT_PROCESSED`.

## 13. Observabilidade

The application exposes a lightweight metrics endpoint at `GET /metrics` and emits JSON logs with correlation context.

Business HTTP requests receive or generate a `correlationId`; SQS processing adds `messageId`, `walletId` and `providerId` when available.

Financial processing, idempotent replays, retries, Outbox publication/failure, reconciliation divergence and processing latency are instrumented through the application metrics port.

Credentials, tokens and complete financial payloads are not logged.

DLQ accounting remains broker-owned: the application records retry/failure activity, while SQS redrive policy is responsible for moving poison messages to the configured DLQ.
