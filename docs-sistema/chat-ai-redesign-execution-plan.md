# Chat AI Redesign Execution Plan

## Current Test Baseline

Command executed from `apps/api`:

```bash
go test ./internal/chat ./internal/availability
```

Result:

```text
ok  	schumacher-tur/api/internal/chat	0.367s
ok  	schumacher-tur/api/internal/availability	(cached)
```

The baseline is green. This is enough to plan safely, but not enough to replace the current prompt fallback with a new JSON decision layer yet. The next implementation slice should add shadow-mode and contract tests before changing production behavior.

## Current Architecture Summary

The current chat flow is centered in `apps/api/internal/chat/service.go`.

1. `Reprocess` loads the session history, selects pending inbound candidates, builds memory, builds agent state, and saves a reprocess snapshot.
2. `conversation_state_machine.go` derives a `CanonicalConversationState` from recent history, booking draft context, and previous tool payloads when `CHAT_CANONICAL_STATE_ENABLED` is enabled.
3. `intent_router.go` runs deterministic intent routing for known cases such as availability search, option selection, passenger count replies, payment, cancellation, unsupported cargo, and SC-MA follow-ups.
4. `response_realizer.go` can answer a small set of deterministic templates without calling the LLM.
5. `tool_router.go` executes backend tools for availability, pricing, booking lookup, booking creation, payment status, payment creation, cancellation, reschedule, and document extraction.
6. `prompt_builder.go` builds a large natural-language system prompt and user prompt with policy text, history, derived context, and tool results.
7. `openai_runner.go` sends the prompt to OpenAI Responses API and returns plain text, with multimodal fallback for image turns.
8. Auto-send policy blocks risky drafts, especially operational claims without tool facts.

The strongest current safety property is that operational truth already comes from backend tools and PostgreSQL-backed repositories. `apps/api/internal/availability/repository.go` searches real trips, stops, prices, status, seats, package names, and date filters from SQL. The model receives these facts, but does not own them.

The weakest current property is that too many business rules are duplicated in natural-language prompts and heuristic parsers. The prompt carries route rules, booking rules, document rules, payment rules, and anti-hallucination instructions. This makes behavior harder to prove and makes prompt size grow as more edge cases are added.

## Prompt-Size Reduction Strategy

Move policy out of the prompt in small slices, keeping the prompt as a voice and ambiguity-resolution aid.

1. Extract stable route policy from `defaultAgentSystemPrompt` into backend tables/constants:
   - supported SC cities and price groups;
   - supported MA cities;
   - SC -> MA and MA -> SC direction rules;
   - unsupported-route handoff rule;
   - public broad-query rules from the SC-MA route note.
2. Extract booking policy into backend state transitions:
   - option selected before passenger collection;
   - passenger count before documents;
   - document confirmation before `booking_create`;
   - booking creation only from backend tool result.
3. Extract payment policy into backend guards:
   - no payment creation without booking;
   - signal vs integral choice before PIX creation;
   - payment confirmation only from payment status/webhook facts.
4. Replace long prompt sections with compact structured facts:
   - `current_turn`;
   - `canonical_state`;
   - `allowed_actions`;
   - `tool_facts`;
   - `missing_fields`;
   - `response_contract`.
5. Keep recent message history short. Prefer canonical state plus the current turn, and include raw history only when ambiguity cannot be resolved deterministically.
6. Add a prompt budget test before enabling compact prompts. The test should fail when prompt length grows beyond a configured limit for common availability, booking, document, and payment turns.

Initial target: keep existing prompts and add compact prompt generation behind `CHAT_COMPACT_PROMPT_ENABLED=false`. Run both in tests/shadow logs before switching.

## Canonical State Strategy

The existing `CanonicalConversationState` should become the backend continuity contract, but it should not become a model-owned memory.

Recommended state shape:

- `RouteState`: origin, destination, package, direction, selected option, trip ID, stop IDs, trip date, departure time.
- `AvailabilityState`: last search input, result count, result IDs, freshness timestamp, source tool call ID.
- `PassengerState`: expected count, child-under-5 count, collected passengers, document confirmation status.
- `BookingState`: booking ID, reservation code, status, source tool call ID.
- `PaymentState`: preference, payment ID, status, amount, source tool call ID/webhook ID.
- `HandoffState`: bot/human status, assigned user, reason.

Rules:

1. Backend tools and database records are the source of truth.
2. Model output can suggest slots or intent, but cannot directly mutate canonical state.
3. Every state transition must pass a validator similar to `validateConversationTransition`.
4. Operational fields must include source metadata when possible: `tool_call_id`, `observed_at`, and `expires_at`.
5. Persist a versioned canonical state snapshot in draft/session metadata before using it for prompt reduction.
6. Keep derivation from history as a fallback during migration, but prefer persisted state once parity tests pass.

## Strict JSON Decision Layer Strategy

Add a strict decision layer only after deterministic routing and before the legacy prompt fallback.

The decision layer should return JSON only, for example:

```json
{
  "intent": "AVAILABILITY_SEARCH",
  "confidence": 0.86,
  "action": "CALL_TOOL",
  "slots": {
    "origin": "Chapeco/SC",
    "destination": "Moncao/MA",
    "package_name": "Pacote p/ Maranhao",
    "trip_date": null,
    "qty": 1
  },
  "missing_fields": ["trip_date"],
  "template": "ASK_TRIP_DATE",
  "reason_code": "sc_origin_ma_destination_follow_up"
}
```

The backend must validate:

1. `intent` is known and allowed for the current canonical phase.
2. `action` is one of a small enum: `ANSWER_TEMPLATE`, `CALL_TOOL`, `ASK_MISSING_FIELD`, `HANDOFF`, `NOOP_REVIEW`.
3. Slots match backend-supported cities, route direction, package, date, quantity, and booking/payment preconditions.
4. Tool calls are created only from validated backend input, never directly from model text.
5. Invalid JSON, low confidence, unsupported slots, or forbidden transitions fall back to deterministic legacy behavior or review.

Rollout order:

1. Build `decision_runner.go` with JSON schema and no production effect.
2. Run it in shadow mode beside `routeDeterministicIntent`.
3. Log disagreements between deterministic and JSON decisions.
4. Add golden tests for disagreements before allowing it to handle any action.
5. Enable only low-risk `ASK_MISSING_FIELD` templates first.
6. Enable `CALL_TOOL` only for read-only tools after validation coverage is strong.

## Tool Execution Boundaries

The backend must remain the only layer allowed to perform business operations.

- Availability claims require an `availability_search` tool result.
- Price claims require `availability_search` or `pricing_quote` facts.
- Booking creation claims require a completed `booking_create` result.
- Payment creation claims require a completed `payment_create` result.
- Payment confirmation requires `payment_status` or webhook-backed payment facts.
- Cancellation claims require `booking_cancel`.
- Reschedule claims require `reschedule_lookup` or the future reschedule mutation tool.

Mutation tools need stricter gates than read-only tools:

1. `booking_create` only when route, selected trip/stops, passenger count, documents, and customer confirmation are valid.
2. `payment_create` only when booking exists and payment type is explicit.
3. `booking_cancel` only when booking/reservation is identified and cancellation intent is explicit.

Do not let the JSON layer or prompt fallback create a tool call unless the backend validator returns a typed input struct.

## Response Template Strategy

Expand templates gradually so operational responses become rendered from facts instead of generated by the LLM.

Current templates cover passenger count, MA origin, MA destination, SC origin, no availability, and unsupported cargo. Keep these and add templates for:

- broad Santa Catarina price table;
- broad Maranhao origin question;
- route missing one field;
- availability result list;
- no availability with alternate-date prompt;
- selected option confirmation;
- passenger document request;
- document extraction confirmation;
- booking created and payment choice;
- PIX created;
- payment status;
- cancellation result;
- human support handoff.

Template data must come from canonical state and tool response payloads. The LLM may polish only non-operational language, and such polishing should remain review-only until tests prove it cannot alter dates, prices, seats, reservation codes, payment status, or route direction.

## Migration Phases

### Phase 0: Baseline and Observability

- Keep current behavior.
- Log prompt size, chosen path, intent decision, canonical phase, tool calls, and auto-send status.
- Add tests for prompt budget and existing deterministic routes.

### Phase 1: Policy Extraction Without Behavior Change

- Move route, booking, document, and payment policy into typed backend constants/functions.
- Keep prompt text but generate parts of it from the same policy source.
- Add tests for SC-MA route policy, unsupported cities, and payment/booking preconditions.

### Phase 2: Canonical State Hardening

- Persist versioned canonical state snapshots.
- Add validators for allowed transitions.
- Add tests proving payment and booking state cannot advance from model-only text.
- Keep history-derived state as fallback.

### Phase 3: JSON Decision Shadow Mode

- Add strict JSON schema and parser.
- Run JSON decisions in shadow mode only.
- Compare JSON decisions against deterministic router decisions and current tool behavior.
- Store disagreement samples for review.

### Phase 4: Template Expansion

- Add templates for availability, booking, payment, and cancellation responses.
- Use templates only when all required facts exist.
- Keep LLM fallback for ambiguous or unsupported cases.

### Phase 5: Compact Prompt Rollout

- Enable compact prompts for a small set of read-only flows.
- Compare auto-send status, draft text, and tool calls against legacy prompt behavior.
- Increase coverage only after golden tests pass.

### Phase 6: Controlled Retirement of Prompt Guardrails

- Remove duplicated natural-language business rules only after the equivalent backend policy and tests exist.
- Keep a legacy prompt rollback flag for at least one deploy cycle after each removal.

## Tests To Add Or Update

Add these before enabling new production behavior:

1. Prompt budget tests for availability, broad SC, broad MA, booking, document image, payment, and cancellation turns.
2. Canonical state persistence tests with versioning and source tool call IDs.
3. Canonical transition tests for invalid booking, payment, cancellation, and route transitions.
4. JSON schema tests for valid decisions, invalid JSON, unknown intents, forbidden actions, unsupported slots, and low confidence.
5. Shadow-decision tests comparing deterministic router output with JSON output for known SC-MA flows.
6. Tool-boundary tests proving no date, price, seats, booking, payment, or cancellation claim can auto-send without matching tool facts.
7. Template tests for every new operational template.
8. Regression tests for existing incremental flow cases:
   - `TestResponseRealizerNoDuplicateMADestinationCase`
   - `TestMADestinationAfterSCOriginExecutesAvailabilityTool`
   - `TestMADestinationAfterSCOriginSkipsResolveAgentToolContext`
   - `TestMADestinationAfterSCOriginDoesNotCallLLMBeforeTool`
   - `TestMaranhaoFlowChapecoThenMoncaoCallsAvailabilitySearchOnce`
9. Availability repository tests for package, city normalization, active trip status, active price status, quantity, and date filters.
10. Auto-send policy tests for JSON/template responses, especially operational claims without facts.

## Risks And Rollback Flags

Existing flags to preserve during migration:

- `CHAT_CANONICAL_STATE_ENABLED`
- `CHAT_INTENT_ROUTER_ENABLED`
- `CHAT_TEMPLATE_REALIZER_ENABLED`
- `CHAT_LLM_INTENT_FALLBACK_ENABLED`
- `CHAT_LEGACY_PROMPT_FALLBACK_ENABLED`

Proposed flags:

- `CHAT_DECISION_JSON_SHADOW_ENABLED`
- `CHAT_DECISION_JSON_ENABLED`
- `CHAT_COMPACT_PROMPT_ENABLED`
- `CHAT_PERSIST_CANONICAL_STATE_ENABLED`
- `CHAT_STRICT_TEMPLATE_RESPONSES_ENABLED`

Primary risks:

1. JSON decision layer misclassifies ambiguous customer text.
2. Persisted canonical state becomes stale and overrides newer backend facts.
3. Compact prompt removes context still needed for edge cases.
4. Templates expose incomplete facts if validators are too permissive.
5. Dual routing creates duplicate tool calls.
6. Mutation tools become easier to trigger accidentally if JSON confidence is trusted too much.

Rollback strategy:

1. Disable `CHAT_DECISION_JSON_ENABLED`.
2. Disable `CHAT_COMPACT_PROMPT_ENABLED`.
3. Disable `CHAT_STRICT_TEMPLATE_RESPONSES_ENABLED`.
4. Keep `CHAT_LEGACY_PROMPT_FALLBACK_ENABLED=true`.
5. Keep deterministic router and existing backend tool guards active.
6. Use stored snapshots, tool calls, and draft payloads to replay failing turns before re-enabling flags.

## Implementation Recommendation

Do not implement the architecture swap yet. The current tests prove recent deterministic SC-MA fixes and several safety gates, but they do not yet prove that a strict JSON layer or compact prompt can replace the legacy prompt fallback safely.

The next code slice should be Phase 0 plus the beginning of Phase 1: prompt budget instrumentation, policy extraction with no behavior change, and tests around route/payment/booking boundaries. Only after that should JSON decision shadow mode be introduced.
