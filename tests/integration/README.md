# Testes de integração PostgreSQL

Os testes deste diretório usam PostgreSQL real e não substituem o banco por mocks.

## Execução

Com PostgreSQL acessível:

```sh
TEST_DATABASE_URL='postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable' \
  go test ./tests/integration -count=1
```

Os testes aplicam as migrations versionadas antes de cada cenário e limpam somente as tabelas do banco de teste.

`TestPostgresDistributedWalletContention` inicia três processos independentes do próprio binário de testes, cada um com pool PostgreSQL separado. `TestPostgresConcurrentIdempotency` inicia 50 processos independentes para a mesma operação e chave de idempotência.

O banco deve ser exclusivo para os testes, pois os cenários executam `TRUNCATE` nas tabelas financeiras.
