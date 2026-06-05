# Hexagonal / Clean Architecture — go-exchange target

Mirrors the `ecomspring` (Spring DDD) reference, adapted to Go idioms. Every
service follows the same four-layer layout under `internal/`. **The Dependency
Rule:** imports point inward only — `domain` imports nothing app-specific;
`application` imports `domain`; `infrastructure` and `interfaces` import
`application` + `domain`; `cmd/main.go` (composition root) wires everything.

```
<service>/internal/
  domain/                      # Enterprise rules. NO gorm/gin/grpc/redis imports.
    <aggregate>.go             #   entity/aggregate + behaviour that enforces invariants
    vo.go                      #   value objects (validated constructors): Currency, Money…
    events.go                  #   domain event payloads (plain structs)
    errors.go                  #   typed/sentinel domain errors (ErrInsufficientBalance…)
    ports.go                   #   repository PORTS (interfaces) in domain terms — ctx only, NO *gorm.DB
  application/                 # Use cases (application rules).
    <usecase>.go               #   interactors orchestrating domain aggregates via ports
    ports.go                   #   OUTBOUND ports: EventPublisher, TxManager, external gateways, cache
    dto.go                     #   command/query inputs + result DTOs (no framework types)
  infrastructure/              # Frameworks & drivers. Implements ports.
    persistence/
      model.go                 #   GORM models (tags) — persistence entities, separate from domain
      mapper.go                #   domain <-> model translation
      *_repo.go                #   repository adapters implementing domain ports
      tx.go                    #   gorm-backed TxManager (tx stored in context)
    eventbus/                  #   EventPublisher adapter over shared/eventbus
    grpcclient/                #   outbound gRPC client adapters (implement application gateways)
    cache/                     #   redis adapters
  interfaces/                  # Inbound adapters (delivery).
    http/                      #   gin handlers (controllers) -> call application use cases
    grpc/                      #   gRPC server handlers -> call application use cases
  cmd/main.go                  # composition root: build infra adapters, inject into use cases, mount interfaces
```

## Conventions
- **Domain entities are pure**: no `gorm`/`json` tags, no `*gorm.DB`. Construct via
  factories that enforce invariants; mutate via behaviour methods (`wallet.Debit(amt)`),
  never by setting fields from outside.
- **Value objects** validate on construction (`domain.NewCurrency("USDT")`).
- **Domain errors** are typed (`var ErrInsufficientBalance = errors.New(...)`), mapped to
  HTTP/gRPC status in the interfaces layer.
- **Repository ports** live in `domain`, expressed with domain types, `context.Context`
  only — never leak `*gorm.DB`. Transactions are coordinated by an `application.TxManager`
  port; the gorm adapter stashes the active `*gorm.DB` in the context and repos read it.
- **Persistence entities** (`infrastructure/persistence/model.go`) carry the gorm tags and
  are mapped to/from domain aggregates by `mapper.go`.
- **Outbound dependencies** (event bus, other services over gRPC, redis cache) are
  `application` ports implemented by `infrastructure` adapters.
- **Interfaces layer** does only transport concerns: decode request → call use case →
  encode response / map domain error to status. No business logic.
- **CQRS-lite**: commands (mutations) and queries (reads) are separate use-case methods;
  heavy read models may bypass the aggregate and project directly in a query service.

## Testing
- **Unit (domain)**: invariants & behaviour, pure, no mocks needed.
- **Unit (application)**: use cases with hand-written fakes/mocks of ports.
- **Integration (infrastructure)**: repositories against an in-memory SQLite
  (`github.com/glebarez/sqlite`, cgo-free) verifying mapper round-trips + queries;
  redis adapters against `miniredis`; event flow against `miniredis` streams.
- **Integration (interfaces)**: gin handlers via `httptest` with real use cases over the
  SQLite repo; gRPC servers via `bufconn`.
```
