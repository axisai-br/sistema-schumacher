# Documentacao da API Backend (Schumacher Tur)

Atualizado em: 2026-04-08

## 1) Visao geral

- Backend HTTP em Go com `chi`.
- Sem prefixo de versao no path. O estado atual usa rotas como `/trips`, `/bookings` e `/payments`.
- A API serve o app interno, fluxos de reservas/pagamentos e rotinas operacionais e administrativas.
- Quase todas as rotas sao protegidas por JWT do Supabase.
- Excecoes publicas: `GET /health`, `GET /ready`, `POST /webhooks/abacatepay`.

Base local padrao:

```txt
http://localhost:<PORT>
```

## 2) Arquitetura e fonte de verdade

Entradas principais do codigo:

- `apps/api/cmd/api/main.go`: bootstrap, middlewares e montagem de rotas.
- `apps/api/internal/shared/config/config.go`: variaveis de ambiente.
- `apps/api/internal/shared/http/decoder.go`: decoder JSON estrito.
- `apps/api/internal/shared/http/response.go`: envelope padrao de resposta e erro.
- `apps/api/internal/*/handler.go`: contratos HTTP por dominio.

Integracoes relevantes:

- JWT do Supabase via JWKS.
- Banco PostgreSQL.
- Pagamentos e webhook publico da AbacatePay.

## 3) Autenticacao

### 3.1 Rotas publicas

- `GET /health`
- `GET /ready`
- `POST /webhooks/abacatepay`

### 3.2 Rotas protegidas

Todas as demais rotas passam pelo middleware de autenticacao.

Header esperado:

```http
Authorization: Bearer <jwt-ou-service-token>
```

Validacoes aplicadas:

- token tecnico configurado em `API_SERVICE_TOKENS`; ou
- token JWT valido via `SUPABASE_JWT_SECRET` para tokens HS256 do Supabase self-hosted, ou via `SUPABASE_JWKS_URL` quando usado;
- `iss` igual ao `SUPABASE_ISSUER` quando configurado;
- `aud` contendo `SUPABASE_AUDIENCE` quando configurado;
- `sub` obrigatorio.

Observacao importante:
- falhas de autenticacao retornam `401` com corpo texto simples via `http.Error`, nao no envelope JSON padrao.
- a autenticacao de servico existe para integracoes server-to-server como `n8n -> API`, evitando depender de `access_token` de sessao do `Supabase Auth` e de fluxo de refresh no workflow.
- `SUPABASE_ALLOW_MISSING_ISSUER=true` e apenas uma compatibilidade temporaria para Supabase Auth emitindo token sem `iss`; ela ainda exige assinatura valida, `exp` valido, `sub`, `role=authenticated` e `aud=authenticated`.

## 4) Convencoes de requisicao

### 4.1 JSON

O backend usa decoder estrito:

- body vazio em endpoints JSON gera erro;
- campos desconhecidos geram erro;
- dados extras apos um JSON valido geram erro.

### 4.2 Datas e horas

- campos `time.Time` devem ser enviados em RFC3339;
- filtros de pagamentos aceitam RFC3339 e tambem `YYYY-MM-DD`.

### 4.3 Paginacao

Padrao de listagem:

- `limit` deve ser `> 0`
- `offset` deve ser `>= 0`

### 4.4 IDs

- a maior parte dos IDs e `UUID`;
- varios handlers validam UUID explicitamente no path.

## 5) Padrao de resposta

Resposta JSON comum:

```json
{
  "qualquer": "payload do endpoint"
}
```

Envelope de erro padrao:

```json
{
  "code": "VALIDATION_ERROR",
  "message": "descricao do erro",
  "details": "opcional"
}
```

Excecoes relevantes:

- middleware de auth responde texto simples;
- alguns fluxos de publicacao/workflow respondem `422` com payload customizado, por exemplo `ROUTE_PUBLISH_BLOCKED` e `ROUTE_NOT_READY`.

## 6) Configuracao e ambiente

### 6.1 Obrigatorias para subir a API

- `DATABASE_URL`
- `SUPABASE_JWT_SECRET` ou `SUPABASE_JWKS_URL`
- `SUPABASE_ISSUER`

### 6.2 Principais opcionais

- `APP_ENV` default `production`
- `PORT` default `8080`
- `CORS_ORIGINS`
- `SUPABASE_URL`
- `SUPABASE_ANON_KEY`
- `SUPABASE_AUDIENCE` default `authenticated`
- `SUPABASE_ALLOW_MISSING_ISSUER` default `false`
- `AUTH_DISABLED`
- `API_SERVICE_TOKENS` lista separada por virgula com tokens tecnicos aceitos no header `Authorization: Bearer ...`
- `ABACATEPAY_API_KEY`
- `ABACATEPAY_WEBHOOK_SECRET`
- `ABACATEPAY_BASE_URL`
- `ABACATEPAY_PUBLIC_KEY`
- `ABACATEPAY_RETURN_URL`
- `ABACATEPAY_COMPLETION_URL`
- `PAGARME_SECRET_KEY`
- `PAGARME_BASE_URL`
- `PAGARME_API_BASE_URL`
- `PAGARME_PAYMENT_RECIPIENT_ID` obrigatoria para `POST /payments` automatico com Pagar.me; o Pix e criado com split de 100% para esse recebedor.
- `PAGARME_DEBUG_RECIPIENT_ID`
- `PAGARME_WEBHOOK_BASIC_USER`
- `PAGARME_WEBHOOK_BASIC_PASS`

### 6.3 CORS

- em desenvolvimento, se `CORS_ORIGINS` estiver vazio e `APP_ENV != production`, a API libera origem dinamica;
- quando configurado, compara `Origin` contra a lista;
- metodos liberados: `GET, POST, PUT, PATCH, DELETE, OPTIONS`.

## 7) Catalogo de endpoints

## 7.1 Infra

### Publicos

- `GET /health`
- `GET /ready`

`/ready` verifica `pool.Ping(...)` no banco e retorna `503 DB_UNAVAILABLE` se a conexao nao estiver pronta.

## 7.2 Usuarios

- `GET /users/me`

Status atual:
- rota protegida;
- implementacao atual responde `501 NOT_IMPLEMENTED`.

## 7.3 Rotas (`/routes`)

Endpoints:

- `GET /routes`
- `POST /routes`
- `GET /routes/cities/candidates`
- `GET /routes/{routeId}`
- `PATCH /routes/{routeId}`
- `POST /routes/{routeId}/publish`
- `POST /routes/{routeId}/duplicate`
- `GET /routes/{routeId}/stops`
- `POST /routes/{routeId}/stops`
- `PATCH /routes/{routeId}/stops/{stopId}`
- `DELETE /routes/{routeId}/stops/{stopId}`
- `GET /routes/{routeId}/segment-prices`
- `PUT /routes/{routeId}/segment-prices`

Query params:

- `GET /routes`: `search`, `status`, `limit`, `offset`
- `status`: `active`, `inactive`, `all`
- `GET /routes/cities/candidates`: `query`, `limit` com teto em `10`

Validacoes e comportamento:

- `POST /routes` exige `name`, `origin_city`, `destination_city`
- erros de geocoding/latitude/longitude caem em `VALIDATION_ERROR`
- `POST /routes/{routeId}/publish` e `PATCH /routes/{routeId}` podem retornar `422` com:

```json
{
  "code": "ROUTE_PUBLISH_BLOCKED",
  "message": "route does not meet publish requirements",
  "requirements_missing": ["..."]
}
```

## 7.4 Viagens (`/trips`)

Endpoints:

- `GET /trips`
- `POST /trips`
- `GET /trips/{tripId}`
- `PATCH /trips/{tripId}`
- `GET /trips/{tripId}/seats`
- `GET /trips/{tripId}/stops`
- `POST /trips/{tripId}/stops`
- `GET /trips/{tripId}/segment-prices`
- `PUT /trips/{tripId}/segment-prices`

Query params:

- `GET /trips`: `search`, `status`, `limit`, `offset`
- `GET /trips/{tripId}/seats`: `board_stop_id` e `alight_stop_id` devem ser enviados juntos quando usados

Validacoes e comportamento:

- `POST /trips` exige `route_id`, `bus_id`, `departure_at`
- `estimated_km` deve ser `>= 0`
- `PATCH /trips/{tripId}` bloqueia mudanca direta de `operational_status`
- rotas de create/update podem retornar `422` com:

```json
{
  "code": "ROUTE_NOT_READY",
  "message": "route is not ready for trip creation or update",
  "requirements_missing": ["..."]
}
```

- `WORKFLOW_LOCKED` aparece quando o workflow controla status operacionais

### Disponibilidade (`/availability`)

Endpoint:

- `GET /availability/search`

Query params:

- `origin`: cidade canonica, por exemplo `Videira/SC`
- `destination`: cidade canonica, por exemplo `Sao Luis/MA`
- `trip_date`: filtro opcional em `YYYY-MM-DD`
- `package_name`: filtro opcional por pacote
- `qtd`: quantidade minima de assentos necessarios; default `1`
- `limit`: default `10`, max `50`
- `only_active`: default `true`
- `include_past`: default `false`

Validacoes e comportamento:

- ao menos um entre `origin`, `destination` ou `package_name` deve ser informado
- `origin` e `destination` aceitam `Cidade/UF`, `Cidade UF`, `Cidade-UF` e `Cidade, UF`; o backend normaliza para `Cidade/UF`
- `origin` e `destination` nao podem ser iguais quando ambos forem enviados
- por padrao a busca retorna apenas viagens futuras com `trip.status in ('SCHEDULED','IN_PROGRESS')`, aceitando tambem `ATIVO` e `ACTIVE` como compatibilidade com carga legada
- por padrao a busca retorna apenas segmentos com `route_segment_prices.status = ACTIVE`
- o payload ja devolve `trip_id`, `board_stop_id`, `alight_stop_id`, `price`, `seats_available` e `package_name`, servindo como agregador para o fluxo `n8n -> API`

Exemplo de integracao `n8n`:

```http
GET /availability/search?origin=Videira/SC&destination=Sao%20Luis/MA&qtd=1 HTTP/1.1
Authorization: Bearer <valor-configurado-em-API_SERVICE_TOKENS>
Accept: application/json
```

Resposta de sucesso:

```json
[
  {
    "segment_id": "trip:board:alight",
    "trip_id": "uuid",
    "route_id": "uuid",
    "board_stop_id": "uuid",
    "alight_stop_id": "uuid",
    "origin_display_name": "Videira/SC",
    "destination_display_name": "Sao Luis/MA",
    "origin_depart_time": "18:30",
    "trip_date": "2026-04-10",
    "seats_available": 12,
    "price": 250.0,
    "currency": "BRL",
    "status": "ACTIVE",
    "trip_status": "SCHEDULED",
    "package_name": "Pacote p/ Maranhao"
  }
]
```

## 7.5 Operacao de viagem

### Trip requests

- `GET /trip-requests`
- `POST /trip-requests`

Query params:
- `limit`, `offset`

### Manifesto

- `GET /trips/{tripId}/manifest`
- `POST /trips/{tripId}/manifest`
- `POST /trips/{tripId}/manifest/sync`
- `PATCH /trips/{tripId}/manifest/{entryId}`

Regras:

- `tripId` e `entryId` sao validados como UUID
- status invalidos retornam `VALIDATION_ERROR`
- item inexistente retorna `404 NOT_FOUND`
- `sync` rematerializa a partir das reservas

### Autorizacoes

- `GET /trips/{tripId}/authorizations`
- `POST /trips/{tripId}/authorizations`
- `PATCH /trips/{tripId}/authorizations/{authorizationId}`

Regras:

- `created_by` pode ser derivado do `sub` autenticado
- erros de autoridade/autorizacao caem em `VALIDATION_ERROR`

### Checklist

- `GET /trips/{tripId}/checklists/{stage}`
- `PUT /trips/{tripId}/checklists/{stage}`

### Driver report

- `GET /trips/{tripId}/driver-report`
- `PUT /trips/{tripId}/driver-report`

### Reconciliation

- `GET /trips/{tripId}/reconciliation`
- `PUT /trips/{tripId}/reconciliation`

### Attachments

- `GET /trips/{tripId}/attachments`
- `POST /trips/{tripId}/attachments`

### Workflow

- `POST /trips/{tripId}/workflow/advance`

Observacoes:

- os blocos acima operam com `tripId` em UUID
- `stage` invalido retorna `VALIDATION_ERROR`
- varios subfluxos retornam `404 NOT_FOUND` quando o recurso ainda nao existe

## 7.6 Reservas (`/bookings`)

Endpoints:

- `GET /bookings`
- `POST /bookings`
- `POST /bookings/checkout`
- `GET /bookings/{bookingId}`
- `PATCH /bookings/{bookingId}`

Query params:

- `GET /bookings`: `booking_id`, `reservation_code`, `trip_id`, `status`, `limit`, `offset`

Payload principal de criacao:

```json
{
  "trip_id": "uuid",
  "seat_id": "uuid opcional",
  "board_stop_id": "uuid",
  "alight_stop_id": "uuid",
  "fare_mode": "AUTO ou MANUAL",
  "fare_amount_final": 350,
  "idempotency_key": "sha256-opcional",
  "passengers": [
    {
      "name": "Maria",
      "document": "00000000000",
      "document_type": "CPF",
      "phone": "48999999999",
      "email": "maria@example.com"
    },
    {
      "name": "Joao",
      "document": "MG1234567",
      "document_type": "RG",
      "phone": "48988888888",
      "email": "joao@example.com"
    }
  ],
  "source": "APP",
  "total_amount": 700,
  "deposit_amount": 150,
  "remainder_amount": 550
}
```

Payload de checkout:

```json
{
  "trip_id": "uuid",
  "seat_id": "uuid opcional",
  "board_stop_id": "uuid",
  "alight_stop_id": "uuid",
  "idempotency_key": "sha256-opcional",
  "passengers": [
    {
      "name": "Maria",
      "document": "00000000000",
      "document_type": "CPF",
      "phone": "48999999999",
      "email": "maria@example.com"
    },
    {
      "name": "Joao",
      "document": "MG1234567",
      "document_type": "RG",
      "phone": "48988888888",
      "email": "joao@example.com"
    }
  ],
  "total_amount": 700,
  "initial_payment": {
    "method": "PIX",
    "amount": 150,
    "description": "Sinal",
    "notes": "",
    "customer": {
      "name": "Maria",
      "email": "maria@example.com",
      "phone": "48999999999",
      "document": "00000000000"
    }
  }
}
```

Validacoes e regras:

- `trip_id`, `board_stop_id`, `alight_stop_id` e pelo menos um `passenger.name` sao obrigatorios
- `passengers[]` e o contrato principal para grupos; `passenger` singular segue aceito como alias retrocompativel para um unico passageiro
- cada passageiro pode informar `document_type` como `CPF`, `RG`, `CNH` ou `CERTIDAO_NASCIMENTO`; aliases operacionais como `CERTIDAO`, `CERTIDAO_DE_NASCIMENTO` e `MATRICULA` sao normalizados para `CERTIDAO_NASCIMENTO`
- quando `document_type` for omitido, a API preserva a compatibilidade anterior e infere `CPF` para documentos com 11 digitos numericos e `RG` nos demais casos
- `idempotency_key` e opcional; quando enviado, a API o persiste junto ao booking para correlacao operacional
- `seat_id` e opcional; quando omitido, a API tenta alocar automaticamente a primeira poltrona livre da viagem
- quando `seat_id` e informado, ele so pode ser usado com um unico passageiro
- a API calcula a tarifa por trecho uma vez e multiplica pelo numero de passageiros para compor `total_amount`
- valores monetarios nao podem ser negativos
- `board_stop_id` precisa vir antes de `alight_stop_id`
- conflito de unicidade de poltrona retorna `409 SEAT_TAKEN`
- `PATCH /bookings/{bookingId}` hoje atualiza apenas `status`
- status aceitos: `PENDING`, `CONFIRMED`, `CANCELLED`, `EXPIRED`

Observacao operacional:

- `GET /bookings` agora permite localizar reserva por `booking_id`, `reservation_code`, `trip_id` e `status`; quando `booking_id` e `reservation_code` vierem juntos, a API trata a busca como `OR` para facilitar integracao com workflows.
- a resposta de `GET /bookings/{bookingId}`, `POST /bookings` e `POST /bookings/checkout` inclui `passengers[]` com `document` e `document_type`; o campo `passenger` continua presente como alias do primeiro passageiro para compatibilidade.
- `POST /bookings` tambem devolve `booking.reservation_code`; o vencimento da reserva segue em `booking.expires_at`.

Resposta importante:

- `POST /bookings/checkout` retorna `booking`, `payment`, `provider_raw`, `checkout_url`, `pix_code`

## 7.7 Pagamentos (`/payments`) e webhook

Endpoints protegidos:

- `GET /payments`
- `POST /payments`
- `POST /payments/manual`
- `GET /payments/{paymentId}/status`
- `POST /payments/{paymentId}/sync`

Endpoint publico:

- `POST /webhooks/abacatepay`

Query params de listagem:

- `booking_id`
- `status`
- `since`
- `until`
- `paid_since`
- `paid_until`
- `limit`
- `offset`

Payload de criacao:

```json
{
  "booking_id": "uuid",
  "amount": 150,
  "method": "PIX",
  "description": "Sinal passagem",
  "customer": {
    "name": "Maria",
    "email": "maria@example.com",
    "phone": "48999999999",
    "document": "00000000000"
  }
}
```

Payload de pagamento manual:

```json
{
  "booking_id": "uuid",
  "amount": 200,
  "method": "CASH",
  "notes": "Pago no embarque"
}
```

Regras:

- `POST /payments` aceita apenas metodos do provedor, como `PIX` e `CARD`.
- `POST /payments/manual` aceita metodos manuais, como `CASH`, `TRANSFER`, `OTHER`.
- `customer.document` em `POST /payments` continua sendo o documento fiscal do pagador (`CPF`/`CNPJ`); `RG`, `CNH` e `CERTIDAO_NASCIMENTO` podem ser usados na reserva, mas nao devem ser reutilizados como `taxId` no provedor.
- pagamentos automaticos Pagar.me exigem `PAGARME_PAYMENT_RECIPIENT_ID`; a order Pix e enviada com `payments[].split` em `type=percentage`, `amount=100` e `recipient_id` do recebedor configurado.
- erro de configuracao de checkout retorna `503 CHECKOUT_NOT_CONFIGURED`.
- `POST /payments` agora devolve `payment`, `provider_raw`, `checkout_url` e `pix_code` quando o provedor retornar esses dados.
- `GET /payments/{paymentId}/status` retorna um resumo com `status`, `amount`, `provider`, `provider_ref`, `metadata`.
- `POST /payments/{paymentId}/sync` retorna `payment`, `booking_status`, `synced`.

Seguranca do webhook:

- aceita `X-Webhook-Secret`
- aceita `Authorization: Bearer <secret>`
- aceita query `webhookSecret=<secret>`
- valida assinatura em `X-AbacatePay-Signature` ou `X-Webhook-Signature` quando configurada

Erros relevantes:

- `401 INVALID_SECRET`
- `401 INVALID_SIGNATURE`
- `400 INVALID_BODY`
- `503 WEBHOOK_NOT_CONFIGURED`

## 7.8 Pricing (`/pricing`)

Endpoints:

- `POST /pricing/quote`
- `GET /pricing/rules`
- `POST /pricing/rules`
- `GET /pricing/rules/{ruleId}`
- `PATCH /pricing/rules/{ruleId}`

Query params de regras:

- `limit`
- `offset`
- `scope`: `GLOBAL`, `ROUTE`, `TRIP`
- `scope_id`
- `rule_type`: `OCCUPANCY`, `LEAD_TIME`, `DOW`, `SEASON`
- `is_active`

Payload de quote:

```json
{
  "trip_id": "uuid",
  "board_stop_id": "uuid",
  "alight_stop_id": "uuid",
  "fare_mode": "AUTO",
  "fare_amount_final": 350
}
```

Resposta de quote:

- `trip_id`, `route_id`, `board_stop_id`, `alight_stop_id`
- `base_amount`, `calc_amount`, `final_amount`
- `currency`
- `fare_mode`
- `occupancy_ratio`
- `applied_rules`
- `snapshot`

Validacoes:

- `trip_id`, `board_stop_id`, `alight_stop_id` obrigatorios
- `scope_id` e obrigatorio quando `scope` for `ROUTE` ou `TRIP`
- `scope_id` deve ser vazio em `GLOBAL`
- `rule_type` e `scope` sao normalizados para uppercase

Erros comuns:

- `TRIP_NOT_FOUND`
- `STOP_NOT_FOUND`
- `INVALID_STOPS`
- `FARE_NOT_FOUND`
- `INVALID_FARE_MODE`
- `FARE_AMOUNT_REQUIRED`

## 7.9 Frota e motoristas

### Buses

- `GET /buses`
- `POST /buses`
- `GET /buses/{busId}`
- `PATCH /buses/{busId}`

Query params:
- `search`, `limit`, `offset`

### Drivers

- `GET /drivers`
- `POST /drivers`
- `GET /drivers/{driverId}`
- `PATCH /drivers/{driverId}`

Query params:
- `search`, `limit`, `offset`

### Driver cards

- `GET /driver-cards`
- `POST /driver-cards`
- `GET /driver-cards/{cardId}`
- `PATCH /driver-cards/{cardId}`
- `POST /driver-cards/{cardId}/block`
- `POST /driver-cards/{cardId}/unblock`
- `GET /driver-cards/{cardId}/transactions`
- `POST /driver-cards/{cardId}/transactions`

Query params:

- `GET /driver-cards`: `driver_id`, `is_active`, `is_blocked`, `limit`, `offset`
- `GET /driver-cards/{cardId}/transactions`: `limit`, `offset`

## 7.10 Financeiro de viagem

### Trip advances

- `GET /trip-advances`
- `POST /trip-advances`
- `GET /trip-advances/{advanceId}`
- `PATCH /trip-advances/{advanceId}`
- `POST /trip-advances/{advanceId}/deliver`

Query params:
- `trip_id`, `driver_id`, `status`, `limit`, `offset`

### Trip expenses

- `GET /trip-expenses`
- `POST /trip-expenses`
- `GET /trip-expenses/{expenseId}`
- `PATCH /trip-expenses/{expenseId}`
- `POST /trip-expenses/{expenseId}/approve`

Query params:
- `trip_id`, `driver_id`, `expense_type`, `payment_method`, `approved`, `limit`, `offset`

### Trip settlements

- `GET /trip-settlements`
- `POST /trip-settlements`
- `GET /trip-settlements/{settlementId}`
- `POST /trip-settlements/{settlementId}/review`
- `POST /trip-settlements/{settlementId}/approve`
- `POST /trip-settlements/{settlementId}/reject`
- `POST /trip-settlements/{settlementId}/complete`

Query params:
- `trip_id`, `driver_id`, `status`, `limit`, `offset`

### Trip validations

- `GET /trip-validations`
- `POST /trip-validations`
- `GET /trip-validations/{validationId}`
- `PATCH /trip-validations/{validationId}`

Query params:
- `trip_id`, `limit`, `offset`

### Advance returns

- `GET /advance-returns`
- `POST /advance-returns`
- `GET /advance-returns/{returnId}`

Query params:
- `trip_advance_id`, `trip_settlement_id`, `limit`, `offset`

### Fiscal documents

- `GET /fiscal-documents`
- `POST /fiscal-documents`
- `GET /fiscal-documents/{documentId}`
- `PATCH /fiscal-documents/{documentId}`

Query params:
- `trip_id`, `document_type`, `status`, `limit`, `offset`

## 7.11 Relatorios

- `GET /reports/passengers`

Query params:

- pelo menos um entre `trip_id`, `trip_date`, `booking_id` e `reservation_code` e obrigatorio
- `include_canceled`: `true` ou `false`, default `false`
- `format`: `json` ou `csv`, default `json`

Comportamento:

- em `json`, retorna linhas por passageiro com `trip_date`, `trip_id`, `booking_id`, `reservation_code`, `seat_number`, `origin`, `destination`, `document`, `document_type`, `booking_status`, `passenger_status`, `total_amount`, `deposit_amount`, `remainder_amount`, `amount_paid` e `payment_stage`
- em `csv`, responde com `Content-Type: text/csv`
- arquivo baixado usa `manifesto.csv`

## 7.12 Almoxarifado e compras

### Suppliers

- `GET /suppliers`
- `POST /suppliers`
- `GET /suppliers/{supplierId}`
- `PATCH /suppliers/{supplierId}`
- `DELETE /suppliers/{supplierId}`

Query params:
- `limit`, `offset`, `active`

### Products

- `GET /products`
- `POST /products`
- `GET /products/{productId}`
- `PATCH /products/{productId}`
- `DELETE /products/{productId}`

Query params:
- `limit`, `offset`, `active`, `category`, `search`

### Service orders

- `GET /service-orders`
- `POST /service-orders`
- `GET /service-orders/{orderId}`
- `PATCH /service-orders/{orderId}`
- `POST /service-orders/{orderId}/start`
- `POST /service-orders/{orderId}/close`
- `POST /service-orders/{orderId}/cancel`
- `DELETE /service-orders/{orderId}`

Query params:
- `limit`, `offset`, `status`, `order_type`, `bus_id`

### Purchase orders

- `GET /purchase-orders`
- `POST /purchase-orders`
- `GET /purchase-orders/{orderId}`
- `PATCH /purchase-orders/{orderId}`
- `POST /purchase-orders/{orderId}/items`
- `DELETE /purchase-orders/{orderId}/items/{itemId}`
- `POST /purchase-orders/{orderId}/send`
- `POST /purchase-orders/{orderId}/receive`
- `POST /purchase-orders/{orderId}/cancel`
- `DELETE /purchase-orders/{orderId}`

Query params:
- `limit`, `offset`, `status`, `supplier_id`, `service_order_id`

### Invoices

- `GET /invoices`
- `POST /invoices`
- `GET /invoices/{invoiceId}`
- `PATCH /invoices/{invoiceId}`
- `POST /invoices/{invoiceId}/process`
- `POST /invoices/{invoiceId}/cancel`
- `DELETE /invoices/{invoiceId}`

Query params:
- `limit`, `offset`, `status`, `supplier_id`, `service_order_id`, `purchase_order_id`, `bus_id`

## 7.13 Importacao XLSX

- `POST /imports/xlsx/upload`
- `POST /imports/xlsx/{batchId}/validate`
- `POST /imports/xlsx/{batchId}/promote`
- `GET /imports/xlsx/{batchId}/report`

Regras:

- upload exige `sheets` no body JSON
- `batchId` vem no path e e validado apenas como string nao vazia no handler
- erros retornam `XLSX_UPLOAD_ERROR`, `XLSX_VALIDATE_ERROR`, `XLSX_PROMOTE_ERROR`, `XLSX_REPORT_ERROR`

## 8) Regras operacionais importantes

- `Supabase` JWT e obrigatorio para quase tudo, mas auth failures nao usam o envelope JSON padrao.
- `routes` e `trips` tem bloqueios de publicacao e readiness com resposta `422`.
- `bookings` protege contra overbooking por conflito de unicidade de poltrona.
- `pricing/quote` e `bookings` dependem da ordem correta entre parada de embarque e desembarque.
- `payments` mistura cobranca via provedor, pagamento manual e sincronizacao posterior.
- `trip_operations` concentra manifesto, autorizacoes, checklist, relatorio do motorista, reconciliacao, anexos e avancos de workflow.

## 9) Gaps e TODOs visiveis no codigo

- `GET /users/me` ainda responde `NOT_IMPLEMENTED`.
- Nao existe prefixo de versao (`/api/v1`) no estado atual.
- O comentario em `payments.RegisterWebhooks` ainda indica ajuste futuro do dominio publico do webhook em producao.
- A documentacao deve ser revisada sempre a partir de `cmd/api/main.go` e `internal/*/handler.go`, porque a API cresceu alem do escopo inicial do plano MVP.
