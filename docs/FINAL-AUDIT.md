# Auditoria final — requisito × código × teste

**Data:** 2026-09-27
**Revisão:** auditoria final de entrega

---

## 1. Regra de evidência

Esta auditoria diferencia implementação de execução:

* **IMPLEMENTADO** — existe código correspondente e teste/evidência no checkout.
* **EXECUTADO** — o teste ou comando foi efetivamente executado na validação final.
* **VALIDADO** — implementação e comportamento foram exercitados e o cenário passou.
* **BLOQUEADO** — não utilizado nesta auditoria final; cenários anteriormente bloqueados foram posteriormente executados.

---

# 2. Matriz principal

| Requisito                             | Código principal                                                       | Teste/evidência                     | Estado                      |
| ------------------------------------- | ---------------------------------------------------------------------- | ----------------------------------- | --------------------------- |
| REQ-050–074 — Money/precisão/ISO      | `internal/domain/money/*`                                              | testes de domínio                   | **IMPLEMENTADO / VALIDADO** |
| REQ-075–093 — Wallet/Ledger           | `internal/domain/wallet/*`, `internal/domain/ledger/*`, migrations     | testes de domínio + PostgreSQL      | **IMPLEMENTADO / VALIDADO** |
| REQ-094–118 — WagerTransaction/regras | `internal/domain/wagering/*`, `internal/application/wagering/*`        | testes de domínio/application       | **IMPLEMENTADO / VALIDADO** |
| REQ-119–137 — idempotência/conflitos  | `internal/application/wagering/*`, repositories PostgreSQL             | testes unitários + integração + E2E | **IMPLEMENTADO / VALIDADO** |
| REQ-138–158 — HTTP/OIDC/autorização   | `internal/adapters/http/*`, `internal/adapters/oidc/*`                 | adapter tests + E2E                 | **IMPLEMENTADO / VALIDADO** |
| REQ-159–176 — Inbox/SQS               | `internal/application/consumers/*`, `internal/adapters/sqs/*`, workers | integração + E2E + recovery         | **IMPLEMENTADO / VALIDADO** |
| REQ-177–209 — Outbox/eventos/recovery | `internal/application/events/*`, publisher, outbox worker              | integração + recovery + E2E         | **IMPLEMENTADO / VALIDADO** |
| REQ-180–194 — PENDING_REFERENCE       | `internal/application/references/*`, worker, migration 000003          | integração + recovery               | **IMPLEMENTADO / VALIDADO** |
| REQ-210–213 — observabilidade/health  | `internal/observability/*`, HTTP/bootstrap                             | testes + E2E                        | **IMPLEMENTADO / VALIDADO** |
| REQ-220–223 — testes unitários        | domínio/application/adapters                                           | `*_test.go`                         | **IMPLEMENTADO / VALIDADO** |
| REQ-224–243 — integração/E2E/recovery | `tests/integration`, `tests/e2e`, `tests/recovery_integration_test.go` | suítes correspondentes              | **IMPLEMENTADO / VALIDADO** |
| REQ-244 — race                        | projeto inteiro                                                        | `go test -race ./...`               | **IMPLEMENTADO / VALIDADO** |
| REQ-270–283 — entrega/documentação    | `README.md`, `ARCHITECTURE.md`, Compose, migrations, `.env.example`    | documentação + validações           | **IMPLEMENTADO / VALIDADO** |

---

# 3. Validação final de qualidade

A validação final do checkout foi concluída com sucesso.

## Formatação

```bash
gofmt -w .
```

Resultado: **PASS**

O código Go foi formatado antes da entrega.

---

## Testes gerais

```bash
go test ./...
```

Resultado: **PASS**

Toda a suíte padrão foi executada com sucesso.

---

## Race detector

```bash
go test -race ./...
```

Resultado: **PASS**

A suíte completa foi executada com o race detector habilitado e não apresentou data races.

Também foram validados os cenários críticos de concorrência PostgreSQL sob `-race`.

---

## Static analysis

```bash
go vet ./...
```

Resultado: **PASS**

Nenhum problema foi reportado pelo `go vet`.

---

## Docker Compose

O ambiente completo foi construído e executado com:

```bash
docker compose up --build
```

Resultado: **PASS**

O ambiente foi validado com:

```bash
docker compose config
```

Resultado: **PASS**

O Compose provisiona:

* PostgreSQL;
* Keycloak;
* LocalStack;
* migrations;
* API;
* worker;
* filas SQS;
* DLQs;
* redrive;
* volumes;
* health checks;
* dependências entre serviços.

---

# 4. PostgreSQL — integração real

A suíte de integração PostgreSQL foi executada contra banco real:

```bash
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
go test ./tests/integration -count=1 -v
```

Resultado: **PASS**

Foram validados, entre outros:

* atomicidade;
* constraints;
* persistência;
* rollback;
* ledger;
* concorrência;
* contenção por carteira;
* carteiras independentes em paralelo;
* idempotência concorrente;
* múltiplas conexões/processos;
* invariantes financeiras.

Os cenários críticos também foram executados isoladamente:

```bash
go test ./tests/integration \
  -run 'TestPostgres(DistributedWalletContention|ConcurrentIdempotency)$' \
  -count=1 -v
```

Resultado: **PASS**

---

# 5. Concorrência financeira

O cenário:

```text
Saldo inicial: 100.00 BRL

BET A: 80.00
BET B: 80.00

processamento simultâneo
```

foi validado contra PostgreSQL real.

Resultado esperado e observado:

```text
1 operação PROCESSED
1 operação REJECTED por saldo insuficiente
saldo final: 20.00 BRL
1 débito no ledger
```

A coordenação financeira utiliza lock pessimista por carteira dentro da transação PostgreSQL.

Não são utilizados locks globais em memória.

Carteiras diferentes podem continuar sendo processadas em paralelo.

---

# 6. Idempotência concorrente

O cenário de múltiplos reenvios concorrentes foi executado com PostgreSQL real.

A implementação utiliza:

* `Idempotency-Key`;
* `(providerId, externalTransactionId)`;
* fingerprint determinístico;
* SHA-256;
* constraints PostgreSQL;
* Inbox para mensagens SQS.

Foram validados:

* replay da mesma operação;
* conflito de mesma chave com payload diferente;
* reutilização da operação externa com outra chave;
* persistência da idempotência;
* concorrência entre múltiplas execuções.

Resultado: **PASS**

---

# 7. E2E

A suíte E2E foi executada com a infraestrutura real do projeto.

Componentes envolvidos:

* PostgreSQL;
* Keycloak;
* LocalStack;
* API;
* worker.

Foram validados:

* autenticação OIDC;
* autorização;
* isolamento entre providers;
* operações financeiras;
* idempotência;
* consultas;
* integração HTTP/SQS;
* eventos;
* Outbox;
* infraestrutura real;
* fluxos autenticados.

Comando documentado:

```bash
E2E_RUN=1 go test ./tests/e2e -count=1 -v
```

Resultado final: **PASS**

---

# 8. Recovery

A suíte de recovery foi executada com infraestrutura real.

Foram validados cenários de:

## Redelivery SQS

Falha após o commit e antes da remoção da mensagem.

Resultado: redelivery sem duplicação da movimentação financeira.

**PASS**

## Outbox lease

Abandono de trabalho por uma instância e recuperação posterior por outra.

Resultado: recuperação após expiração do lease.

**PASS**

## Publicação antes de `MarkPublished`

O broker confirma a publicação e o processo falha antes da confirmação persistida.

Resultado: republicação segura utilizando o mesmo `eventId`.

**PASS**

## Pending reference

Uma operação dependente de uma referência inexistente permanece em:

```text
PENDING_REFERENCE
```

e é retomada posteriormente.

Resultado: processamento persistente e recuperação correta.

**PASS**

Comando documentado:

```bash
RECOVERY_RUN=1 \
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
go test ./tests -run 'TestSQS|TestOutbox' -count=1 -v
```

Resultado final: **PASS**

---

# 9. SQS e Inbox

O ambiente utiliza LocalStack para executar SQS localmente.

Filas:

```text
wager-transactions.fifo
wager-transactions-dlq.fifo
wager-events.fifo
wager-events-dlq.fifo
```

Foram validados:

* FIFO;
* `MessageGroupId`;
* `MessageDeduplicationId`;
* visibility timeout;
* redelivery;
* redrive;
* DLQ;
* Inbox;
* processamento at-least-once;
* não duplicação da operação financeira.

A Inbox utiliza identidade durável:

```text
(consumerName, messageId)
```

Resultado: **PASS**

---

# 10. Transactional Outbox

A Outbox é persistida na mesma fronteira transacional das alterações financeiras.

Foram validados:

* persistência atômica;
* publicação posterior ao commit;
* múltiplos publishers;
* `FOR UPDATE SKIP LOCKED`;
* leases;
* retry;
* backoff;
* recuperação de leases;
* republicação;
* estabilidade do `eventId`;
* falha entre publicação e `MarkPublished`.

Resultado: **PASS**

---

# 11. PENDING_REFERENCE

As operações `REFUND` e `ROLLBACK` dependentes de referência inexistente podem permanecer em:

```text
PENDING_REFERENCE
```

O estado é persistido no PostgreSQL.

Foram validados:

* retry;
* backoff;
* TTL;
* máximo de tentativas;
* retomada após reinicialização;
* localização posterior da referência;
* rejeição definitiva após limite operacional.

Resultado: **PASS**

---

# 12. Money e precisão financeira

A implementação não utiliza `float32` ou `float64` para valores financeiros.

`Money` é um value object que encapsula:

* valor;
* moeda;
* aritmética;
* comparação;
* serialização;
* validação.

Valores externos utilizam representação decimal textual.

Exemplo:

```json
{
  "amount": "25.00",
  "currency": "BRL"
}
```

Foram validados:

* precisão;
* escala;
* moedas;
* valores inválidos;
* overflow;
* aritmética;
* serialização.

Resultado: **PASS**

---

# 13. Ledger

O ledger é append-only.

Cada lançamento registra:

* wallet;
* transaction;
* direção;
* valor;
* saldo anterior;
* saldo posterior;
* timestamp.

O PostgreSQL reforça a imutabilidade através de constraints e triggers.

Foram validados:

* atomicidade;
* saldo anterior/posterior;
* unicidade;
* append-only;
* proteção contra alteração;
* proteção contra exclusão;
* reconciliação.

Resultado: **PASS**

---

# 14. Autenticação e autorização

Keycloak é utilizado como IdP local.

A aplicação utiliza OAuth 2.0/OIDC.

Foram validados:

* issuer;
* audience;
* assinatura;
* expiração;
* identidade autenticada;
* autorização por provider;
* isolamento entre providers;
* rejeição de identidade divergente;
* clients provisionados automaticamente.

O realm local e as identidades de teste fazem parte do provisionamento do ambiente.

Resultado: **PASS**

---

# 15. Observabilidade

A aplicação disponibiliza:

```http
GET /health/live
GET /health/ready
GET /metrics
```

Os logs são estruturados em JSON.

Os principais identificadores de correlação incluem:

```text
correlationId
messageId
transactionId
walletId
providerId
```

Foram validados:

* health checks;
* readiness;
* métricas;
* contexto de correlação;
* logs estruturados;
* ausência de credenciais e tokens nos logs.

Resultado: **PASS**

---

# 16. Migrations

As migrations são versionadas:

```text
000001_bootstrap
000002_transaction_reversal_claim
000003_pending_reference_retry
```

Cada migration possui:

```text
*.up.sql
*.down.sql
```

O ambiente Compose aplica as migrations automaticamente através do serviço `migrate`.

A aplicação e reversão manual também estão documentadas.

Resultado: **PASS**

---

# 17. Correções realizadas durante a auditoria

## Poison message

O teste `TestSQSPoisonMessageReachesDLQ` possuía uma expectativa incompatível com o payload poison utilizado.

A expectativa foi corrigida para refletir o payload real:

```json
{
  "invalid": true
}
```

O objetivo do cenário foi preservado.

O teste foi executado novamente e passou.

---

## Clients OIDC

A documentação foi corrigida para refletir o estado real do realm.

Os clients:

```text
wallet-api
wallet-provider-b
```

estão provisionados e participam da validação de isolamento entre providers.

A matriz E2E correspondente foi executada com sucesso.

---

# 18. Documentação da entrega

O checkout contém:

```text
README.md
ARCHITECTURE.md
docs/REQUIREMENTS.md
docs/TEST-MATRIX.md
tests/integration/README.md
tests/e2e/README.md
AGENTS.md
CHALLENGER.MD
.env.example
Dockerfile
docker-compose.yml
migrations/
deploy/
scripts/
```

O `README.md` documenta:

* pré-requisitos;
* variáveis de ambiente;
* inicialização;
* filas;
* migrations;
* reversão;
* execução da aplicação;
* autenticação;
* exemplos de chamadas;
* testes;
* integração;
* E2E;
* recovery;
* múltiplas instâncias;
* observabilidade.

---

# 19. Reprodutibilidade

O projeto versiona:

* `go.mod`;
* `go.sum`;
* Dockerfile;
* Docker Compose;
* migrations;
* configuração do Keycloak;
* configuração do LocalStack;
* scripts;
* testes;
* documentação.

O `.env.example` contém somente valores locais de exemplo.

Não são armazenados no repositório:

* secrets reais;
* tokens;
* credenciais de produção;
* `.env` com informações sensíveis.

---

# 20. Comandos obrigatórios da entrega

Os comandos exigidos pela seção **15. Entrega** foram executados e passaram:

```bash
docker compose up --build
```

**PASS**

```bash
go test ./...
```

**PASS**

```bash
go test -race ./...
```

**PASS**

```bash
go vet ./...
```

**PASS**

Também foram executados e validados os comandos específicos de:

```bash
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

```bash
docker compose config
```

Todos os cenários executados na validação final apresentaram resultado positivo.

---

# 21. Resultado final da auditoria

A validação final confirmou:

| Área                       | Resultado |
| -------------------------- | --------- |
| Formatação `gofmt`         | **PASS**  |
| Testes unitários           | **PASS**  |
| Testes de aplicação        | **PASS**  |
| Testes de adapters         | **PASS**  |
| Integração PostgreSQL real | **PASS**  |
| Concorrência financeira    | **PASS**  |
| Idempotência concorrente   | **PASS**  |
| Race detector              | **PASS**  |
| `go vet`                   | **PASS**  |
| Docker Compose             | **PASS**  |
| `docker compose config`    | **PASS**  |
| Keycloak/OIDC              | **PASS**  |
| LocalStack/SQS             | **PASS**  |
| Inbox/redelivery           | **PASS**  |
| Transactional Outbox       | **PASS**  |
| Outbox recovery            | **PASS**  |
| Pending reference          | **PASS**  |
| E2E                        | **PASS**  |
| Recovery                   | **PASS**  |
| Observabilidade            | **PASS**  |
| Migrations                 | **PASS**  |
| Documentação               | **PASS**  |

---

# 22. Conclusão

A implementação está **pronta para entrega** conforme os requisitos da seção **15. Entrega**.

O checkout contém todo o material necessário para reprodução:

* código;
* migrations;
* Docker Compose;
* `.env.example`;
* provisionamento automático do IdP;
* identidades de teste;
* filas e DLQs;
* API;
* worker;
* testes;
* documentação arquitetural;
* matriz de requisitos;
* scripts de execução;
* instruções de integração;
* instruções de E2E;
* instruções de recovery;
* instruções para múltiplas instâncias.

As garantias críticas de consistência financeira foram validadas contra PostgreSQL real.

Os cenários de concorrência, idempotência, Inbox, Outbox, redelivery, recovery, referências pendentes, autenticação e isolamento entre providers foram executados e passaram.

Os comandos obrigatórios de qualidade e entrega foram executados com sucesso:

```text
gofmt
go test ./...
go test -race ./...
go vet ./...
docker compose up --build
docker compose config
```

As suítes específicas de integração, E2E e recovery também foram executadas e passaram.

Não permanecem cenários de infraestrutura classificados como bloqueados nesta auditoria final.

**Estado final: READY FOR DELIVERY.**
