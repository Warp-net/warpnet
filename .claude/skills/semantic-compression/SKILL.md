---
name: semantic-compression
description: >
  Apply semantic logic compression (Ovchinnikov method) when crafting prompts for LLMs or describing complex conditional logic. Use this skill whenever the user asks to: write or optimise a prompt for an LLM, compress or rewrite a task description, describe multi-condition business logic (if/and/or/not chains), or asks why an LLM misunderstood their instructions. Also trigger when the user says ‘compress the logic’, ‘write a prompt for’, ‘the LLM doesn't understand’, ‘simplify the conditions’, or similar intents. Also trigger when the user wants to use output modifiers such as →code or →no-meta, or domain prefixes such as Dev:, Biz:, Sec:.
---

# Semantic Logic Compression (Information Compression Protocol)

Convert natural language logic into structured symbolic form that LLMs parse more reliably.
This is NOT summarization — it preserves all conditions while making structure explicit.

## When to apply

- Multi-condition logic: "if A and B and not C, then D"
- Business rules with temporal constraints ("within 5 minutes", "last 30 days")
- Any prompt where missed conditions would break the expected output
- Code generation tasks where logic ambiguity causes junior-style output
- When the user needs to radically save context tokens (up to 70-80% reduction).

## Core symbol set

### 1. Logical Operators
| Natural language     | Symbol | Example                          |
|----------------------|--------|----------------------------------|
| and              | `∧`    | `платёж OK ∧ товар есть`         |
| or             | `∨`    | `admin ∨ moderator`              |
| if...then| `→`    | `T > 30 → включить кондиционер`  |
| not             | `¬`    | `¬отмена за 5 мин`               |
| then and only then | `↔`    | `доступ ↔ верифицирован`         |
| for all             | `∀`    | `∀ item → validate(item)`        |
| exists           | `∃`    | `∃ active_session`               |

### 2. Structural Markers & Modifiers
Use these markers to categorize constraints and instructions.

| Type                 | Marker / Symbol | Description / Example                          |
|----------------------|-----------------|------------------------------------------------|
| **Domain** | `Dev:`, `Ops:`  | Prefix for the task domain (e.g., `Dev:`, `Biz:`, `Sec:`) |
| **Requirements** | `Req:`          | Mandatory system requirements                  |
| **Technology Stack** | `Stack:`        | e.g., `Stack: Py/µserv/PG`                     |
| **Tone/Output** | `→no-meta`      | No AI self-references or fluff                 |
| **Output Format** | `→code`, `→tab` | Output only pure code or a table               |
| **Priorities** | `✓`, `✓✓`, `~`  | Required (`✓`), Critical (`✓✓`), Optional (`~`)|

## Compression process

1. **Extract** — identify all conditions and actions from the natural language
2. **Label** — give each condition a short alias if needed (`P = платёж OK`)
3. **Encode & Structure** — replace connectives with symbols, preserve temporal/quantitative constraints, and group context using structural markers (`Req:`, `Stack:`).
4. **Strip** — remove filler words, keep only logical skeleton
5. **Verify** — mentally re-read: does the compressed form cover all original cases?

## Output format

Always show both versions side by side so the user can verify nothing was lost:

```text
Original:
  "Write me a Python function that queries the MongoDB database, retrieves users from it—provided their `active` flag is set to `true` and they aren’t banned—and send me the clean code without any fluff."

Compressed:
  Dev: →code; Stack: Py/MongoDB; Req: (active=true ∧ ¬ban); →no-meta; →no-safe
```

## What NOT to do
Don't compress non-conditional descriptive text (it won't help)

Don't drop temporal constraints — they're often the part LLMs miss most

Don't use compression as a substitute for clear variable naming

If logic is simple (1-2 conditions), natural language is fine — say so

Don't invent your own markers outside the Information Compression Protocol.

## Example: code generation prompt
Before:
```"Write a function that confirms an order if the item is in stock, payment has been successfully processed, and the order has not been cancelled within the last 5 minutes."```

After:
```
Biz: →code;
Impl: confirm_order(order) → bool
Cond: order.has_stock ∧ order.payment_ok ∧ ¬order.cancelled[Δt ≤ 5min]
Return True only if all three conditions are met.
```
