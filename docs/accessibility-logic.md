# Accessibility Data Logic

This document describes how InWheel handles accessibility data, inheritance between parent and child places, and write-time validation.

## 1. Core Principles

1. **Data fidelity:** The API stores what is submitted. It does not compute whether a place is accessible — that judgment belongs to the client, which knows the user's specific needs.
2. **Facts over opinions:** `AuditFlags` on a component are objective facts derived from submitted property values (e.g. entrance width < 0.8m is a measurable fact). They are stored for clients to use, not for the server to act on.
3. **Specific overrides general:** A child place's own component data always takes precedence over the parent's equivalent component.
4. **Self-contradictions are rejected at write time:** Data that directly contradicts itself (e.g. a component marked `accessible` but containing a physical barrier the submitter described) is rejected with HTTP 422. Everything else is accepted.

## 2. Inheritance and Overrides

InWheel supports a hierarchical parent-child relationship (e.g. a mall containing shops).

### Effective Profile

When querying a child place, the system can compute an **effective profile** by merging the child's own components with the parent's. The child inherits any component type the parent has that the child does not provide itself.

Inherited components are marked `is_inherited: true` and carry the `source_id` of the parent place.

### Child Overrides Parent

If a child provides its own data for a component type, the parent's data for that same type is ignored entirely for that child's view. There is no partial merging within a component.

Example: a mall has an `accessible` entrance; a shop inside has its own `inaccessible` entrance. The shop's effective profile shows only its own `inaccessible` entrance.

### No Bottom-Up Propagation

A child's data never modifies the parent's record. Each place is its own source of truth.

## 3. AuditFlags

On every write (POST or PATCH with accessibility data), `a11y.Engine.WithAuditFlags()` runs synchronously and populates `AuditFlags` on each component based on the submitted property values:

| Component | Flag | Condition |
|---|---|---|
| Entrance | `narrow width (0.8m required)` | `width < 0.8m` |
| Entrance | `contains step` | `has_step = true` |
| Entrance | `high step (>0.05m)` | `has_step = true` and `step_height > 0.05m` |
| Entrance | `step with no ramp` | `has_step = true` and `has_ramp = false` |
| Restroom | `not wheelchair accessible` | `wheelchair_accessible = false` |
| Restroom | `narrow door (0.8m required)` | `door_width < 0.8m` |
| Restroom | `missing grab rails` | `grab_rails = false` |
| Elevator | `small cabin width (0.8m required)` | `width < 0.8m` |
| Elevator | `small cabin depth (1.1m required)` | `depth < 1.1m` |
| Elevator | `missing braille` | `braille = false` |
| Elevator | `missing audio` | `audio = false` |
| Parking | `no disabled spaces` | `has_disabled_spaces = false` |

These flags are stored on the component and returned in API responses. Clients use them to evaluate relevance for a specific user's needs.

## 4. Write-time Validation

Flags fall into two categories that determine whether a write is accepted or rejected.

### Hard Contradiction Flags (reject with HTTP 422)

These flags represent cases where the submitter's own data directly contradicts the submitted `overall_status`. The data is internally inconsistent regardless of any accessibility opinion.

| Flag | Why it's a hard contradiction |
|---|---|
| `step with no ramp` | The submitter described a physical barrier with no workaround, then marked the component accessible |
| `not wheelchair accessible` | The submitter explicitly stated it is not wheelchair accessible, then marked it accessible |
| `no disabled spaces` | The submitter explicitly stated there are no disabled spaces, then marked parking accessible |

A 422 response includes the list of conflicts:

```json
{
  "error": "accessibility data contains conflicts",
  "conflicts": [
    { "component": "entrance", "reason": "status is accessible but: step with no ramp" }
  ]
}
```

### Informational Flags (stored, never block)

All other flags (narrow width, high step, missing braille, etc.) are based on measurement thresholds derived from accessibility standards. They are stored as facts for clients to interpret. A place with a 0.75m entrance can still be marked `accessible` — whether that matters depends on the user's wheelchair width, which the server does not know.

## 5. What the Server Does Not Do

- It does not compute a general accessibility rating for a place.
- It does not decide whether a set of flags makes a place inaccessible.
- It does not modify `overall_status` based on component data.

These decisions belong to client applications, which can filter and rank places based on the specific accessibility needs of their users.
