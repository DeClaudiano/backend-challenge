# Test Matrix

| Área | Suíte/comando | Infraestrutura | Evidência no repositório | Execução P4 |
|---|---|---|---|---|
| Unit/application | `go test ./...` | Go modules | `*_test.go` | PASSOU |
| Race | `go test -race ./...` | Go modules | testes unitários/application | PASSOU|
| Static | `go vet ./...` | Go modules | código Go | PASSOU|
| PostgreSQL | `go test ./tests/integration -count=1 -v` | PostgreSQL real | `tests/integration` | PASSOU |
| E2E | `E2E_RUN=1 go test ./tests/e2e -count=1 -v` | PostgreSQL + Keycloak + LocalStack | `tests/e2e` | PASSOU|
| Recovery | `RECOVERY_RUN=1 TEST_DATABASE_URL=... go test ./tests -run 'TestSQS|TestOutbox' -count=1 -v` | PostgreSQL + LocalStack | `tests/recovery_integration_test.go` | PASSOU|
| Compose | `docker compose config` / `docker compose up --build -d` | Docker Compose | `docker-compose.yml` | PASSOU |
| Shell/JSON | `sh -n deploy/compose/localstack-init.sh scripts/migrate.sh` + validação de `realm.json` | shell/JSON | scripts/realm | PASSOU |
| Formatting | `gofmt -l .` | Go | código Go | PASSOU: nenhum arquivo pendente |

## Critério

Um cenário só é marcado como **EXECUTADO/APROVADO** após execução bem-sucedida no ambiente correspondente. **IMPLEMENTADO** e **PREPARADO** não equivalem a execução.

## Infraestrutura real

O challenge exige PostgreSQL, IdP e SQS-compatible infrastructure reais para os cenários de integração/E2E. Mocks/doubles permanecem úteis para testes unitários de contratos, mas não substituem essa evidência.

## Estado 

Concluida
