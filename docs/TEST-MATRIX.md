# Test Matrix

| Área | Suíte/comando | Infraestrutura | Evidência no repositório | Execução P4 |
|---|---|---|---|---|
| Unit/application | `go test ./...` | Go modules | `*_test.go` | BLOQUEADA: dependências Go não concluíram download no ambiente |
| Race | `go test -race ./...` | Go modules | testes unitários/application | BLOQUEADA pelo mesmo ambiente |
| Static | `go vet ./...` | Go modules | código Go | BLOQUEADA pelo mesmo ambiente |
| PostgreSQL | `go test ./tests/integration -count=1 -v` | PostgreSQL real | `tests/integration` | NÃO EXECUTADA: Docker indisponível |
| E2E | `E2E_RUN=1 go test ./tests/e2e -count=1 -v` | PostgreSQL + Keycloak + LocalStack | `tests/e2e` | NÃO EXECUTADA: Docker indisponível |
| Recovery | `RECOVERY_RUN=1 TEST_DATABASE_URL=... go test ./tests -run 'TestSQS|TestOutbox' -count=1 -v` | PostgreSQL + LocalStack | `tests/recovery_integration_test.go` | NÃO EXECUTADA: Docker indisponível |
| Compose | `docker compose config` / `docker compose up --build -d` | Docker Compose | `docker-compose.yml` | NÃO EXECUTADA: Docker indisponível |
| Shell/JSON | `sh -n deploy/compose/localstack-init.sh scripts/migrate.sh` + validação de `realm.json` | shell/JSON | scripts/realm | PASSOU |
| Formatting | `gofmt -l .` | Go | código Go | PASSOU: nenhum arquivo pendente |

## Critério

Um cenário só é marcado como **EXECUTADO/APROVADO** após execução bem-sucedida no ambiente correspondente. **IMPLEMENTADO** e **PREPARADO** não equivalem a execução.

## Infraestrutura real

O challenge exige PostgreSQL, IdP e SQS-compatible infrastructure reais para os cenários de integração/E2E. Mocks/doubles permanecem úteis para testes unitários de contratos, mas não substituem essa evidência.

## Estado P4

A matriz de código e testes está presente no checkout. Nesta auditoria P4, a execução completa permaneceu bloqueada por duas limitações do ambiente: Docker não está instalado/disponível e o `go test ./...` não conseguiu concluir o download das dependências dentro do tempo disponível. Portanto, nenhuma suíte dependente de infraestrutura real é declarada como aprovada sem execução.
