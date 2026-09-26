# Checklist rastreável de requisitos

**Fonte de verdade:** `challenge.md`. O conteúdo foi lido integralmente e esta checklist usa a referência padronizada `challenge.md`.

**Status de auditoria:** os itens abaixo continuam marcados como `[ ]` porque esta checklist é a matriz de rastreabilidade original e não foi usada como mecanismo automático de aprovação. **Isso não significa que os itens estejam ausentes.** O estado técnico atual e as lacunas verificadas estão consolidados em `ARCHITECTURE.md` e no `README.md`. Marque um item somente quando houver evidência correspondente reproduzível; requisitos opcionais não contam como obrigatórios.

**Última auditoria do checkout:** 2026-09-26.

## 1. Objetivo e garantias gerais

- [ ] **REQ-001** Implementar serviço em Go com Uber Fx para operações financeiras de provedores de jogos. — §1, L1–9
- [ ] **REQ-002** Expor API HTTP e consumidor de mensagens que movimentem carteiras com garantias equivalentes. — §1, L5–9
- [ ] **REQ-003** Demonstrar correção financeira com várias instâncias e falhas entre etapas. — §1, L7–9
- [ ] **REQ-004** Priorizar precisão monetária, integridade do ledger, idempotência persistente, concorrência, recuperação e decisões arquiteturais. — §1, L7–9

## 2. Autenticação e autorização

- [ ] **REQ-010** Exigir autenticação e autorização reais integradas a IdP externo OAuth 2.0/OIDC. — §2, L11–19
- [ ] **REQ-011** Justificar em `ARCHITECTURE.md` escolha do IdP, validação de credenciais e modelo de permissões. — §2, L13–15
- [ ] **REQ-012** Usar Keycloak no Docker Compose e `client_credentials` para comunicação serviço a serviço ou documentar/justificar a escolha de IdP e credenciais. — §2, L15
- [ ] **REQ-013** Não implementar cadastro próprio de senhas nem emissão própria de tokens. — §2, L15
- [ ] **REQ-014** Derivar `providerId` autorizado da identidade autenticada. — §2, L17
- [ ] **REQ-015** Isolar transações por provedor, inclusive em consultas e replays. — §2, L17
- [ ] **REQ-016** Restringir operações de carteira ao serviço interno. — §2, L17
- [ ] **REQ-017** Controlar acesso à mensageria por credenciais e políticas do broker, mantendo validações de domínio no consumidor. — §2, L19

## Nota de interpretação de autorização

`challenge.md` determina que providers acessam apenas suas próprias transações e que operações de carteira são restritas ao serviço interno. Como o texto não separa expressamente os GET de carteira/ledger dessa última categoria, a arquitetura adota a interpretação conservadora de restringir as consultas `/wallets` ao serviço interno; a ambiguidade permanece registrada em `ARCHITECTURE.md`.

## 3. Operações, entrega e falhas distribuídas

- [ ] **REQ-020** Cada operação externa identifica jogador, carteira, jogo, provedor e rodada. — §3, L21–34
- [ ] **REQ-021** Suportar os tipos externos `BET`, `WIN`, `LOSS`, `REFUND` e `ROLLBACK`. — §3, L23
- [ ] **REQ-022** Assumir entrega at-least-once. — §3, L25–34
- [ ] **REQ-023** Tratar recebimento repetido da mesma operação, inclusive por HTTP e SQS. — §3, L25–34
- [ ] **REQ-024** Tratar reversão que chega antes da transação referenciada. — §3, L25–34
- [ ] **REQ-025** Tratar operações simultâneas sobre a mesma carteira. — §3, L25–34
- [ ] **REQ-026** Permanecer correto após encerramento abrupto antes ou depois de commit. — §3, L25–34
- [ ] **REQ-027** Tratar publicação repetida de evento de integração. — §3, L25–34
- [ ] **REQ-028** Tratar indisponibilidade temporária de PostgreSQL ou SQS. — §3, L25–34
- [ ] **REQ-029** Em todos esses cenários, impedir movimentação duplicada, saldo negativo e perda de evento cujo registro foi confirmado no banco. — §3, L34

## 4. Stack, acesso ao banco e ciclo de vida

- [ ] **REQ-030** Usar Go e declarar a versão em `go.mod` e Dockerfile. — §4, L36–51
- [ ] **REQ-031** Versionar `go.mod` e `go.sum` e utilizar Go Modules. — §4, L42–43
- [ ] **REQ-032** Usar Uber Fx (`go.uber.org/fx`) para composição. — §4, L44; L59–70
- [ ] **REQ-033** Usar `net/http` ou roteador Go. — §4, L45
- [ ] **REQ-034** Usar IdP externo OAuth 2.0/OIDC. — §4, L46
- [ ] **REQ-035** Usar PostgreSQL. — §4, L47
- [ ] **REQ-036** Usar AWS SQS executado localmente com LocalStack ou MiniStack. — §4, L48
- [ ] **REQ-037** Usar Docker Compose para ambiente local. — §4, L49
- [ ] **REQ-038** Manter migrations versionadas e documentar aplicação e reversão. — §4, L50
- [ ] **REQ-039** Usar `testing`/`go test`, incluindo execução com `-race`. — §4, L51
- [ ] **REQ-040** Escolher biblioteca de acesso ao PostgreSQL; `pgx` com SQL explícito é preferencial, `sqlc` opcional, `database/sql`/GORM aceitos. — §4, L53–57
- [ ] **REQ-041** Manter transações, locks e constraints explícitos e verificáveis. — §4, L55
- [ ] **REQ-042** Documentar em `ARCHITECTURE.md` biblioteca de banco, representação de `Money` e fronteira da transação SQL entre repositórios. — §4, L57
- [ ] **REQ-043** Compor configuração, conexões, repositórios, casos de uso, handlers e workers usando Fx, injeção por construtores, `fx.Module`, `fx.Provide` e `fx.Invoke`. — §4, L59–61
- [ ] **REQ-044** Gerenciar servidor, workers e recursos com `fx.Lifecycle`. — §4, L63–68
- [ ] **REQ-045** Validar configuração e dependências na inicialização. — §4, L65
- [ ] **REQ-046** Implementar cancelamento, prazos e término observável dos workers. — §4, L66
- [ ] **REQ-047** No shutdown, parar novas entradas e concluir ou liberar trabalho em andamento. — §4, L67
- [ ] **REQ-048** Fechar dependências somente depois dos componentes que as utilizam. — §4, L68
- [ ] **REQ-049** Manter domínio independente de Fx, HTTP, SQS e bibliotecas de persistência. — §4, L70

## 5. Garantias obrigatórias

- [ ] **REQ-050** Não converter valores monetários a `float32`/`float64` em parsing, cálculo, serialização ou persistência. — §5, L72–81
- [ ] **REQ-051** Persistir idempotência de modo que sobreviva ao reinício de todos os processos. — §5, L75
- [ ] **REQ-052** Garantir invariantes financeiras no banco, independentemente de locks locais e deduplicação SQS FIFO. — §5, L77–81
- [ ] **REQ-053** Publicar eventos externos somente após confirmação da transação que os originou. — §5, L77
- [ ] **REQ-054** Manter ledger append-only; corrigir finanças com novos lançamentos. — §5, L78
- [ ] **REQ-055** Permitir avanço paralelo de carteiras independentes e proibir lock global. — §5, L79
- [ ] **REQ-056** Impedir lost updates em alterações de saldo. — §5, L80
- [ ] **REQ-057** Impor unicidade, não negatividade e imutabilidade do ledger no schema, constraints e mecanismos de proteção do banco. — §5, L81

## 6. Modelo de domínio

### Encapsulamento, erros e I/O

- [ ] **REQ-060** Encapsular estado das entidades, usar construtores com validação e métodos explícitos de transição. — §6, L83–93
- [ ] **REQ-061** Preservar invariantes em todas as operações públicas. — §6, L87
- [ ] **REQ-062** Separar criação de reidratação; reidratar sem reaplicar movimentações, transições ou eventos. — §6, L89
- [ ] **REQ-063** Rejeitar valores de domínio não inicializados ou inválidos. — §6, L91
- [ ] **REQ-064** Tornar erros de domínio classificáveis por tipo ou `errors.Is`/`errors.As`; não usar `panic` para rejeições de negócio. — §6, L93
- [ ] **REQ-065** Fazer operações de I/O receberem `context.Context` e respeitarem cancelamento e timeout. — §6, L93

### `Money`

- [ ] **REQ-066** Modelar `Money` como value object imutável de valor e moeda. — §6.1, L95–110
- [ ] **REQ-067** Suportar criação por string decimal, zero por moeda, soma, subtração, negação, comparação e serialização. — §6.1, L97
- [ ] **REQ-068** Usar unidades mínimas em `int64` ou biblioteca decimal de precisão exata; documentar representação e limites. — §6.1, L99
- [ ] **REQ-069** Receber e devolver valor externo no formato `{"amount":"25.00","currency":"BRL"}`. — §6.1, L101
- [ ] **REQ-070** Usar escala fixa de duas casas e código ISO 4217. — §6.1, L102
- [ ] **REQ-071** Rejeitar valores vazios, `NaN`, `Infinity`, notação científica, escala excedente e valores negativos nas entradas financeiras externas. — §6.1, L103
- [ ] **REQ-072** Não arredondar silenciosamente entrada inválida; se formas equivalentes forem aceitas, documentar normalização anterior ao hash de idempotência. O texto mostra `25.00` e exige escala fixa de duas casas, mas não declara literalmente se `25` é aceito como forma equivalente; essa decisão permanece uma ambiguidade de contrato. — §6.1, L101–104
- [ ] **REQ-073** Exigir moedas compatíveis em aritmética e comparação. — §6.1, L105
- [ ] **REQ-074** Se usar `int64`, tratar overflow em parsing, soma, subtração e negação. — §6.1, L106
- [ ] **REQ-075** Permitir valores negativos em diferenças/cálculos internos, mas não em saldo. — §6.1, L107
- [ ] **REQ-076** Preservar exatamente valor e moeda na persistência, por exemplo em unidades mínimas `BIGINT` ou `NUMERIC`. — §6.1, L108
- [ ] **REQ-077** Moeda BRL exclusiva nos cenários principais é permitida somente se o tipo ainda carregar moeda e houver teste de incompatibilidade entre moedas. — §6.1, L110

### `Wallet`

- [ ] **REQ-078** Fazer carteira a raiz do agregado financeiro com identidade, jogador, moeda, saldo, versão e instantes de criação/atualização. — §6.2, L112–125
- [ ] **REQ-079** Expor criação, reidratação, débito e crédito; manter alteração do saldo sob controle do agregado e transação SQL. — §6.2, L116
- [ ] **REQ-080** Impor unicidade de `(playerId,currency)`. — §6.2, L118
- [ ] **REQ-081** Preservar saldo maior ou igual a zero após débito. — §6.2, L119
- [ ] **REQ-082** Exigir que a moeda da movimentação coincida com a moeda da carteira. — §6.2, L120
- [ ] **REQ-083** Confirmar mudança financeira e lançamento correspondente no ledger juntos. — §6.2, L121
- [ ] **REQ-084** Iniciar versão em `1` e incrementá-la após criação somente quando houver mudança de saldo. — §6.2, L122
- [ ] **REQ-085** Não descartar atualizações confirmadas em disputas entre escritores. — §6.2, L123
- [ ] **REQ-086** Documentar estratégia de concorrência. — §6.2, L125

### `WagerTransaction`

- [ ] **REQ-087** Suportar tipos `OPENING`, `BET`, `WIN`, `LOSS`, `REFUND`, `ROLLBACK`. — §6.3, L127–149
- [ ] **REQ-088** Para operações externas, registrar identificadores interno/externo, provedor, chave de idempotência, hash do payload, carteira, jogador, rodada, jogo, tipo, `Money`, referência externa opcional, estado e timestamps. — §6.3, L131
- [ ] **REQ-089** Persistir também referência interna resolvida, código de falha e resultado financeiro retornado ao provedor quando aplicável. — §6.3, L131
- [ ] **REQ-090** Iniciar em `PENDING`; validar pelo domínio transições para processamento, espera por referência, rejeição e falha permanente. — §6.3, L133
- [ ] **REQ-091** Implementar significados dos estados: `PENDING`, `PENDING_REFERENCE`, `PROCESSED`, `REJECTED`, `FAILED`, com os três últimos terminais. — §6.3, L135–143
- [ ] **REQ-092** Impedir novas transições em estado terminal; replay consulta resultado persistido sem reaplicar operação. — §6.3, L143
- [ ] **REQ-093** Documentar máquina de estados e distinguir falhas transitórias de permanentes. — §6.3, L143
- [ ] **REQ-094** Dar retomada durável por outra instância a todo `PENDING` confirmado. — §6.3, L145
- [ ] **REQ-095** Permitir conclusão síncrona sem commit intermediário de aceite para operações sem dependências. — §6.3, L145
- [ ] **REQ-096** Reservar `OPENING` à abertura interna e rejeitá-lo em HTTP/SQS. — §6.3, L147
- [ ] **REQ-097** Exigir para `OPENING` identidade interna estável, carteira, jogador, moeda, valor, estado e timestamps; não aplicar metadados externos inaplicáveis. — §6.3, L149
- [ ] **REQ-098** Distinguir no schema operações internas e externas e impedir crédito inicial duplicado. — §6.3, L149

### Ledger, Inbox e Outbox

- [ ] **REQ-099** Registrar em cada lançamento `id`, `walletId`, `transactionId`, direção (`DEBIT`/`CREDIT`), valor, saldo anterior/posterior e instante. — §6.4, L151–157
- [ ] **REQ-100** Tornar o lançamento imutável e validar `balanceAfter = balanceBefore ± money` conforme direção. — §6.4, L155–157
- [ ] **REQ-101** Impor unicidade `(walletId,transactionId)` no banco e proteção contra edição/exclusão. — §6.4, L157
- [ ] **REQ-102** Não criar ledger para `LOSS` nem operações rejeitadas; ledger de partidas dobradas é opcional. — §6.4, L157
- [ ] **REQ-103** Persistir Inbox com identidade de mensagem/consumidor, hash, recebimento e conclusão; impor unique `(consumerName,messageId)`. — §6.5, L159–166
- [ ] **REQ-104** Persistir Outbox com ID estável, agregado, tipo, payload, ocorrência, tentativas, próximo envio e publicação; suportar retry com backoff. — §6.5, L163–166
- [ ] **REQ-105** Na entrada SQS, gravar inbox/conclusão durável na mesma transação SQL que alterações de domínio, ledger e eventos correspondentes. — §6.5, L166
- [ ] **REQ-106** Para referência pendente, permitir concluir mensagem após persistir pendência, passando continuidade ao worker de referências. — §6.5, L166

## 7. Regras das operações e referências

- [ ] **REQ-110** `BET`: débito, valor positivo e saldo suficiente. — §7, L168–186
- [ ] **REQ-111** `WIN`: crédito com valor positivo; pode referenciar uma aposta da mesma rodada. — §7, L172–173
- [ ] **REQ-112** `LOSS`: sem movimentação, exige `money.amount` igual a `"0.00"`, não cria ledger nem altera versão. — §7, L174, L182
- [ ] **REQ-113** Ao processar `LOSS`, emitir `WagerTransactionProcessed`, sem `WalletBalanceChanged`. — §7, L182
- [ ] **REQ-114** `REFUND`: crédito integral do valor de `BET` processada. — §7, L175
- [ ] **REQ-115** `ROLLBACK`: movimento contrário integral a `BET`, `WIN` ou `REFUND` processada. — §7, L176
- [ ] **REQ-116** Exigir `referenceExternalTransactionId` em `REFUND`/`ROLLBACK` e resolver por `(providerId,referenceExternalTransactionId)`. — §7, L178
- [ ] **REQ-117** Exigir concordância entre operação e referência quanto a provedor, jogador, carteira, moeda e rodada; valor de reversão igual ao referenciado; não suportar reversão parcial. — §7, L180
- [ ] **REQ-118** Permitir zero no saldo inicial e em `LOSS`; exigir valor maior que zero em `BET`, `WIN`, `REFUND`, `ROLLBACK`. — §7, L182
- [ ] **REQ-119** Continuar exigindo em `LOSS` a moeda da carteira. — §7, L182
- [ ] **REQ-120** Impedir duas reversões bem-sucedidas do mesmo tipo para uma referência. — §7, L184
- [ ] **REQ-121** Documentar interação de `REFUND` e `ROLLBACK` sobre a mesma aposta e impedir devolução duplicada do mesmo débito. — §7, L184
- [ ] **REQ-122** Rejeitar e auditar reversão que debite acima do saldo disponível, com failure code diferente do código de aposta sem saldo. — §7, L186
- [ ] **REQ-123** Persistir como `PENDING_REFERENCE` referência ainda ausente; retry de worker com backoff exponencial, inclusive após reinício. — §7, L188–194
- [ ] **REQ-124** Definir máximo de tentativas ou TTL; no esgotamento finalizar `REJECTED`, com código de referência não encontrada e evento de rejeição. — §7, L192
- [ ] **REQ-125** Documentar comportamento se referência existe mas está pendente ou terminou sem sucesso. — §7, L192
- [ ] **REQ-126** Toda rejeição deve ter `failureCode` estável e documentado, distinguindo entradas corrigíveis de resultados definitivos. — §7, L194

## 8. Concorrência

- [ ] **REQ-130** Coordenar concorrência por carteira, usando locking pessimista, controle otimista com retry limitado, atualização atômica condicionada ou combinação justificada. — §8, L196–200
- [ ] **REQ-131** Demonstrar garantias com pelo menos três processos independentes, cada um com conexões e memória próprias. — §8, L200
- [ ] **REQ-132** Testar simultaneamente duas apostas distintas de `80.00 BRL` em carteira com `100.00 BRL`. — §8, L202–204
- [ ] **REQ-133** No cenário anterior obter exatamente uma aposta processada, uma rejeitada por saldo insuficiente, saldo final `20.00 BRL` e um débito no ledger. — §8, L204
- [ ] **REQ-134** Confirmar que reenvios não alteram o resultado da disputa. — §8, L204
- [ ] **REQ-135** Continuar processando carteiras diferentes em paralelo. — §8, L204

## 9. Contratos HTTP

### Carteira e consultas

- [ ] **REQ-140** Implementar `POST /wallets` com `playerId` e `initialBalance` no formato Money documentado. — §9, L206–233
- [ ] **REQ-141** Devolver abertura com `id`, `playerId`, `balance`, `version`. — §9, L222–231
- [ ] **REQ-142** Abertura com saldo positivo cria `OPENING` `PROCESSED`, lançamento de crédito e outbox de `WagerTransactionProcessed` e `WalletBalanceChanged` no mesmo commit da carteira. — §9, L233
- [ ] **REQ-143** Eventos de abertura interna não exigem metadados externos inaplicáveis; versão inicial da carteira é `1`. — §9, L233
- [ ] **REQ-144** Abertura com saldo inicial zero não cria `OPENING`, ledger nem eventos financeiros. — §9, L233
- [ ] **REQ-145** Impedir segunda carteira para mesmo jogador/moeda e responder conflito. — §9, L233
- [ ] **REQ-146** Implementar `GET /wallets/:walletId`. — §9, L235–244
- [ ] **REQ-147** Implementar `GET /wallets/:walletId/ledger?cursor=...&limit=50`. — §9, L237–244
- [ ] **REQ-148** Paginar ledger com cursor opaco e ordenação estável. — §9, L244
- [ ] **REQ-149** Implementar `GET /wagering/transactions/:transactionId`. — §9, L237–244
- [ ] **REQ-150** Implementar `GET /providers/:providerId/wagering/transactions/:externalTransactionId`. — §9, L241–244
- [ ] **REQ-151** Permitir acompanhar pendências e consultar códigos de rejeição/falha em consultas de transação. — §9, L244

### Envio, chave, replay e contrato de erros

- [ ] **REQ-152** Implementar `POST /wagering/transactions` e o contrato dos campos apresentados (`providerId`, ID externo, jogador, carteira, rodada, jogo, tipo e money). — §9, L246–278
- [ ] **REQ-153** Exigir header `Idempotency-Key`; não substituir silenciosamente por chave calculada. — §9, L278–280
- [ ] **REQ-154** Aceitar `referenceExternalTransactionId` em operações de reversão. — §9, L278
- [ ] **REQ-155** Persistir hash determinístico dos campos de negócio em JSON canônico com chaves ordenadas; excluir chave de idempotência e metadados de transporte. — §9, L282
- [ ] **REQ-156** Documentar algoritmo, campos e normalizações do hash, garantindo equivalência HTTP/SQS. — §9, L282
- [ ] **REQ-157** Chave e conteúdo equivalentes retornam resultado persistido e `idempotentReplay: true`. — §9, L284
- [ ] **REQ-158** Chave reutilizada com conteúdo diferente devolve conflito. — §9, L285
- [ ] **REQ-159** Impedir reaplicação de mesma operação financeira identificada por `(providerId,externalTransactionId)` sob outra chave. — §9, L286
- [ ] **REQ-160** Replay de operação concluída devolve saldo observado no processamento original, ainda que carteira tenha mudado depois. — §9, L287
- [ ] **REQ-161** Documentar códigos HTTP e corpos diferenciando entrada inválida, conflito, rejeição de negócio, processamento pendente e indisponibilidade transitória. — §9, L289

### Reconciliação e health

- [ ] **REQ-162** Implementar `POST /wallets/:walletId/reconciliation`. — §9, L291–312
- [ ] **REQ-163** Reconstruir saldo pelo ledger, incluindo abertura, e comparar com saldo armazenado em visão consistente. — §9, L310
- [ ] **REQ-164** Calcular `difference = storedBalance - calculatedBalance` e reportar `consistent`/`checkedEntries`. — §9, L299–310
- [ ] **REQ-165** Reportar divergências na resposta, logs e métrica; reconciliação não altera saldo. — §9, L312
- [ ] **REQ-166** Implementar health checks públicos `GET /health/live` e `GET /health/ready`. — §9, L314–321
- [ ] **REQ-167** Liveness representa processo e readiness verifica PostgreSQL e SQS. — §9, L321

## 10. Consumidor SQS

- [ ] **REQ-170** Provisionar `wager-transactions.fifo` e `wager-transactions-dlq.fifo`, incluindo redrive. — §10, L323–327
- [ ] **REQ-171** Suportar envelope/tipo `WagerTransactionRequested` e os campos de dados exemplificados. — §10, L329–346
- [ ] **REQ-172** Compartilhar entre HTTP/SQS caso de uso e garantias de idempotência financeira. — §10, L348
- [ ] **REQ-173** Usar `data.idempotencyKey` em SQS e deduplicar adicionalmente pela inbox. — §10, L348
- [ ] **REQ-174** Usar `messageId` do envelope como identidade durável para o consumidor e verificar hash em reentregas. — §10, L350
- [ ] **REQ-175** Remover mensagem somente depois do commit do tratamento durável. — §10, L351
- [ ] **REQ-176** Tratar rejeições de negócio confirmadas como terminais e permitir remoção da mensagem. — §10, L352
- [ ] **REQ-177** Aplicar retry/backoff a falhas transitórias; enviar erros permanentes ou tentativas esgotadas à DLQ. — §10, L353
- [ ] **REQ-178** Documentar máximo de tentativas, visibility timeout e mensagens inválidas; manter `maxReceiveCount`, visibility timeout, TTL, backoff e demais parâmetros operacionais configuráveis, pois o desafio não fixa seus valores. — §10, L353–355
- [ ] **REQ-179** Em `SIGTERM`, parar polling e concluir trabalho no prazo ou liberar visibilidade para reentrega segura. — §10, L355
- [ ] **REQ-180** Documentar `MessageGroupId`, `MessageDeduplicationId` e validar concorrência entre HTTP e SQS. — §10, L357

## 11. Outbox e eventos

- [ ] **REQ-190** Confirmar atomicamente estado da operação, saldo, ledger, inbox e registros de eventos conforme aplicável. — §11, L359–367
- [ ] **REQ-191** Usar worker separado para publicar outbox pendente. — §11, L363
- [ ] **REQ-192** Suportar múltiplos publishers, disputa por registros, backoff e recuperação de trabalho abandonado. — §11, L363
- [ ] **REQ-193** Recuperar interrupção entre commit e publicação e entre publicação e confirmação da outbox. — §11, L365
- [ ] **REQ-194** Fazer eventos pendentes serem assumidos por outra instância e preservar `eventId` em republicações. — §11, L365
- [ ] **REQ-195** Provisionar destino de eventos e documentar contratos de roteamento e consumo. — §11, L367
- [ ] **REQ-196** Emitir `WagerTransactionProcessed` na conclusão bem-sucedida, incluindo `LOSS`. — §11, L369–375
- [ ] **REQ-197** Emitir `WagerTransactionRejected` na rejeição definitiva por regra de negócio. — §11, L373–375
- [ ] **REQ-198** Emitir `WalletBalanceChanged` em alteração efetiva do saldo. — §11, L375
- [ ] **REQ-199** Emitir `WagerTransactionPendingReference` ao registrar espera pela referência. — §11, L375
- [ ] **REQ-200** Definir tipos concretos por evento e envelope com `eventId`, `eventType`, `aggregateId`, `correlationId`, `causationId` opcional, `occurredAt`, `version` e `data` tipado. — §11, L378
- [ ] **REQ-201** Definir tipo/versão no construtor do evento; usar timestamps UTC RFC 3339, dinheiro como strings decimais e snapshot imutável na outbox. — §11, L382
- [ ] **REQ-202** Incluir em `WalletBalanceChanged`: `walletId`, `transactionId`, `direction`, `money`, `balanceBefore`, `balanceAfter`, `walletVersion`. — §11, L380

## 12. Observabilidade

- [ ] **REQ-210** Emitir logs JSON com identificadores disponíveis para rastrear: `correlationId`, `messageId`, `transactionId`, `walletId`, `providerId`. — §12, L384–390
- [ ] **REQ-211** Não registrar credenciais, dados sensíveis ou payloads financeiros completos. — §12, L386
- [ ] **REQ-212** Expor métricas de resultados por status, duplicatas, retries, DLQ, conflitos de concorrência, atraso da outbox, latência de processamento e divergências de reconciliação. — §12, L388
- [ ] **REQ-213** Incluir health checks definidos na API. — §12, L390
- [ ] **REQ-214** Tracing OpenTelemetry e dashboards são diferenciais opcionais. — §12, L390

## 13. Verificação e testes obrigatórios

### Unitários

- [ ] **REQ-220** Testar parsing e operações de `Money`, escala, limites numéricos, entradas inválidas e moedas incompatíveis. — §13, L392–396
- [ ] **REQ-221** Testar invariantes da carteira e transições de estado. — §13, L396
- [ ] **REQ-222** Testar regras dos cinco tipos externos, política de valor zero de cada tipo e conflito de payload para mesma chave. — §13, L396
- [ ] **REQ-223** Testar abertura interna, metadados e eventos correspondentes. — §13, L396

### Integração, E2E e autenticação

- [ ] **REQ-224** Executar PostgreSQL, IdP e LocalStack ou MiniStack em containers reais. — §13, L398–402
- [ ] **REQ-225** Testar migrations, constraints e imutabilidade do ledger. — §13, L400
- [ ] **REQ-226** Testar atomicidade financeira, inbox, reentrega, concorrência da outbox, retry, DLQ e recuperação após reinicialização. — §13, L400
- [ ] **REQ-227** Testar composição Fx, startup e shutdown, incluindo liberação de recursos dos workers. — §13, L402
- [ ] **REQ-228** Não substituir toda a infraestrutura por mocks. — §13, L402
- [ ] **REQ-229** Testar integração real com IdP e rejeição de credenciais ausentes, inválidas ou expiradas. — §13, L404–408
- [ ] **REQ-230** Testar isolamento entre provedores em consultas e replays e restrição das operações internas. — §13, L407
- [ ] **REQ-231** Verificar ausência de efeitos financeiros ou exposição de dados em acessos não autorizados. — §13, L408

### Concorrência e recuperação

- [ ] **REQ-232** Enviar a mesma aposta 50 vezes em paralelo e comprovar um único débito. — §13, L410–425
- [ ] **REQ-233** Executar disputa das duas apostas de `80.00` sobre saldo `100.00`. — §13, L413
- [ ] **REQ-234** Processar carteiras distintas simultaneamente. — §13, L414
- [ ] **REQ-235** Repetir cenários relevantes com pelo menos três instâncias independentes. — §13, L415
- [ ] **REQ-236** Interromper consumidor após commit e antes da remoção; validar reentrega. — §13, L416
- [ ] **REQ-237** Executar dois publishers disputando a mesma outbox e validar recuperação de publicação. — §13, L417
- [ ] **REQ-238** Entregar `REFUND`/`ROLLBACK` antes da referência e comprovar resolução posterior ou rejeição por expiração. — §13, L418
- [ ] **REQ-239** Reiniciar a aplicação e comprovar preservação de idempotência, pendências e consistência financeira. — §13, L419
- [ ] **REQ-240** Se houver aceite assíncrono, interromper após confirmar `PENDING` e antes de executar; outra instância deve retomar. — §13, L419
- [ ] **REQ-241** Conferir saldo armazenado contra créditos menos débitos do ledger. — §13, L421
- [ ] **REQ-242** Incluir cenários da mesma operação cruzando HTTP e SQS. — §13, L421
- [ ] **REQ-243** Exercitar duplicidade recebida repetidamente para testar deduplicação da aplicação, não apenas deduplicação FIFO. — §13, L423
- [ ] **REQ-244** Executar `go test -race` nos testes aplicáveis. — §13, L425

## 14. Critérios de avaliação, eliminatórios e diferenciais

- [ ] **REQ-250** Evidenciar integridade financeira (20 pontos): precisão, invariantes, reversões e reconciliação confiáveis. — §14, L427–439
- [ ] **REQ-251** Evidenciar concorrência (20 pontos): coordenação entre processos sem atualizações perdidas. — §14, L432
- [ ] **REQ-252** Evidenciar idempotência (15 pontos): persistência, detecção de conflito e reprodução do resultado original. — §14, L433
- [ ] **REQ-253** Evidenciar mensageria/recuperação (15 pontos): inbox, outbox, retries, DLQ e encerramento seguro. — §14, L434
- [ ] **REQ-254** Evidenciar modelagem/arquitetura (10 pontos): encapsulamento em Go, Fx, autenticação e autorização. — §14, L435
- [ ] **REQ-255** Evidenciar testes (10 pontos): integração real, isolamento de provedores, paralelismo e interrupção. — §14, L436
- [ ] **REQ-256** Evidenciar observabilidade (5 pontos): diagnóstico por logs, métricas e health checks. — §14, L437
- [ ] **REQ-257** Evidenciar documentação (5 pontos): execução reproduzível e decisões técnicas explicadas. — §14, L438–439
- [ ] **REQ-258** Evitar todos os critérios eliminatórios: falta de auth efetiva em endpoints de negócio; acesso não autorizado a operações/transações; cálculo monetário em ponto flutuante; saldo negativo por concorrência; movimentação duplicada; idempotência só em memória; dependência de instância única; publicação anterior ao commit; ausência de ledger auditável; ou substituição integral de PostgreSQL/SQS/IdP por mocks. — §14, L441
- [ ] **REQ-259 (opcional)** Partidas dobradas são diferencial opcional. — §14, L443
- [ ] **REQ-260 (opcional)** Tracing é diferencial opcional. — §14, L443
- [ ] **REQ-261 (opcional)** Testes de carga são diferenciais opcionais; se realizados, documentar comando reproduzível, ambiente, metodologia, throughput, p50/p95/p99, erros, conflitos e atraso da outbox. — §14, L443
- [ ] **REQ-262** Não inventar meta mínima de RPS; o desafio não define uma. — §14, L443

## 15. Entrega e documentação

- [ ] **REQ-270** Entregar código, migrations, Docker Compose e instruções para reproduzir a partir de checkout limpo. — §15, L445–466
- [ ] **REQ-271** README da solução explica pré-requisitos e variáveis de ambiente. — §15, L449
- [ ] **REQ-272** README explica inicialização das filas, aplicação/reversão de migrations, execução da aplicação, exemplos de chamadas e comandos de teste. — §15, L449
- [ ] **REQ-273** Incluir `.env.example` com valores locais de exemplo e sem segredos reais. — §15, L449
- [ ] **REQ-274** Provisionar automaticamente o IdP e identidades de teste e documentar como executar fluxos autenticados. — §15, L451
- [ ] **REQ-275** Registrar em `ARCHITECTURE.md` decisões sobre dinheiro, transações, idempotência, locks, referências pendentes, reversões, inbox/outbox, autenticação, autorização, Fx e shutdown. — §15, L453
- [ ] **REQ-276** Explicitar em `ARCHITECTURE.md` limitações, interpretações adotadas e trabalho não concluído. — §15, L453
- [ ] **REQ-277** Disponibilizar `docker compose up --build` ou equivalente documentado. — §15, L455–462
- [ ] **REQ-278** Disponibilizar `go test ./...` ou equivalente documentado. — §15, L459–462
- [ ] **REQ-279** Disponibilizar `go test -race ./...` ou equivalente documentado. — §15, L460–462
- [ ] **REQ-280** Disponibilizar `go vet ./...` ou equivalente documentado. — §15, L461–462
- [ ] **REQ-281** Documentar separadamente preparação de dependências de teste, integração, múltiplas instâncias e simulações de falha. — §15, L464
- [ ] **REQ-282** Documentar comandos correspondentes se forem usados build tags. — §15, L464
- [ ] **REQ-283** Entregar código formatado com `gofmt` e dependências reproduzíveis. — §15, L466

## Decisões não normativas mantidas fora da checklist

A política arquitetural de permitir somente uma reversão direta bem-sucedida por transação consumida (`REFUND` ou `ROLLBACK`) é uma interpretação necessária para impedir devolução financeira duplicada; `challenge.md` exige a coerência dessa combinação, mas não prescreve sozinho toda a matriz de combinações. TTL, visibility timeout, `maxReceiveCount`, backoff e outros parâmetros operacionais também não são requisitos fixos e permanecerão configuráveis.

## Notas de rastreabilidade

- IDs são estáveis para facilitar referências em issues/PRs e na matriz futura de testes; mudanças de redação do requisito não devem reutilizar o ID para outro requisito.
- `ARCHITECTURE.md` documenta escolhas propostas para requisitos que pedem uma estratégia ou justificativa; isso não marca os requisitos como implementados.
- A especificação pede também `docs/TEST-MATRIX.md` no README e evidência de testes. A matriz de testes detalhada não é criada nesta Fase 0, pois o pedido desta fase limita os artefatos a `ARCHITECTURE.md` e `docs/REQUIREMENTS.md`.
