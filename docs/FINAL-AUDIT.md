# Auditoria final — requisito × código × teste

Data: 2026-09-26 — revisão P4

## Regra de evidência

`IMPLEMENTADO` significa que há código correspondente e teste(s) no checkout.
`EXECUTADO` significa que o teste foi efetivamente executado nesta revisão.
`BLOQUEADO` significa que existe teste/implementação, mas a execução exige infraestrutura ou dependências indisponíveis neste ambiente.

## Matriz principal

| Requisito | Código principal | Teste/evidência | Estado |
|---|---|---|---|
| REQ-050–074 — Money/precisão/ISO | `internal/domain/money/*` | `internal/domain/money/money_test.go` | IMPLEMENTADO; testes focados executados |
| REQ-075–093 — Wallet/Ledger | `internal/domain/wallet/*`, `internal/domain/ledger/*`, migrations | `internal/domain/*_test.go`, `tests/integration/postgres_integration_test.go` | IMPLEMENTADO; integração real bloqueada nesta máquina |
| REQ-094–118 — WagerTransaction/regras | `internal/domain/wagering/*`, `internal/application/wagering/*` | `internal/domain/wagering/*_test.go`, `internal/application/wagering/*_test.go` | IMPLEMENTADO |
| REQ-119–137 — idempotência/conflitos | `internal/application/wagering/*`, `transaction_repository.go` | `services_test.go`, `tests/integration/postgres_integration_test.go`, E2E | IMPLEMENTADO; integração/E2E bloqueados |
| REQ-138–158 — HTTP/OIDC/autorização | `internal/adapters/http/*`, `internal/adapters/oidc/*` | adapter tests + `tests/e2e/e2e_test.go` | IMPLEMENTADO; E2E bloqueado |
| REQ-159–176 — Inbox/SQS | `internal/application/consumers/*`, `internal/adapters/sqs/*`, `worker.go` | consumer tests + `tests/recovery_integration_test.go` + E2E | IMPLEMENTADO; infraestrutura real bloqueada |
| REQ-177–209 — Outbox/eventos/recovery | `internal/application/events/*`, `publisher/*`, `outbox_repository.go`, `outbox_worker.go` | event/publisher tests + recovery/E2E | IMPLEMENTADO; infraestrutura real bloqueada |
| REQ-180–194 — PENDING_REFERENCE | `internal/application/references/*`, `reference_worker.go`, migration 000003 | application tests + PostgreSQL integration/recovery | IMPLEMENTADO; execução real bloqueada |
| REQ-210–213 — observabilidade/health | `internal/observability/*`, HTTP/worker bootstrap | metrics/context tests + HTTP E2E | IMPLEMENTADO; E2E bloqueado |
| REQ-220–223 — unitários | domínio/application/adapters | `*_test.go` | IMPLEMENTADO; foco executável depende de módulos |
| REQ-224–243 — integração/E2E/recovery | `tests/integration`, `tests/e2e`, `tests/recovery_integration_test.go` | matriz real no repositório | IMPLEMENTADO; EXECUÇÃO BLOQUEADA |
| REQ-244 — race | projeto inteiro | `go test -race ./...` | BLOQUEADO nesta máquina |
| REQ-270–283 — entrega/documentação | `README.md`, `ARCHITECTURE.md`, Compose, migrations, `.env.example` | `docs/TEST-MATRIX.md`, scripts e documentação | IMPLEMENTADO; execução limpa bloqueada |

## Verificações locais realizadas nesta revisão

- `gofmt -l` — sem arquivos pendentes.
- `sh -n deploy/compose/localstack-init.sh scripts/migrate.sh` — passou.
- `deploy/keycloak/realm.json` — JSON válido.
- `go test ./...` — não concluído: dependências Go não terminaram de baixar no ambiente.
- `go test -race ./...` — não executado pelo mesmo bloqueio.
- `go vet ./...` — não executado pelo mesmo bloqueio.
- `docker compose config` — não executado: Docker não está instalado/disponível no ambiente.
- PostgreSQL/Keycloak/LocalStack reais — não executados nesta revisão.

## Correção encontrada na auditoria

O teste `TestSQSPoisonMessageReachesDLQ` verificava um `messageId` dentro de um payload deliberadamente inválido que não continha esse campo. O teste foi corrigido para validar o payload poison real (`{"invalid":true}`), preservando o objetivo do cenário.

Também foi corrigida a inconsistência documental de `ARCHITECTURE.md`: a documentação dizia que ainda não existiam dois clients de provider para a prova E2E, embora `wallet-api` e `wallet-provider-b` já estivessem provisionados.

## Conclusão P4

A implementação permanece **estruturalmente fechada para P0–P3** e o P4 atualizou a documentação para refletir o estado real do checkout, sem alterar código de produção já validado.

A validação final **não pode ser declarada 100% executada nesta máquina**. Durante P4: `gofmt -l .` passou; validações de shell/JSON passaram; `go test ./...` foi iniciado, mas o download das dependências Go não concluiu dentro do limite disponível; Docker não está instalado/disponível, portanto PostgreSQL, Keycloak, LocalStack, Compose, E2E e recovery real não puderam ser executados.

Para fechar a evidência em uma máquina com Docker e acesso ao proxy Go, executar exatamente:

1. `docker compose up --build -d`;
2. `go test ./...`;
3. `go test -race ./...`;
4. `go vet ./...`;
5. `go test ./tests/integration -count=1 -v`;
6. `E2E_RUN=1 go test ./tests/e2e -count=1 -v`;
7. `RECOVERY_RUN=1 TEST_DATABASE_URL=... go test ./tests -run 'TestSQS|TestOutbox' -count=1 -v`;
8. verificar `/health/live`, `/health/ready`, `/metrics` e shutdown limpo dos containers.

Somente após esses comandos passarem a matriz deve ser marcada como `EXECUTADA/APROVADA`.
