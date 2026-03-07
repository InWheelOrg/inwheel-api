# Accessibility Data Logic

This document describes how InWheel handles accessibility data, inheritance between parent and child places, and the
role of the LLM Auditor in resolving logical contradictions.

## 1. Core Principles

The InWheel accessibility engine is designed with three core principles:

1. **Raw Data Fidelity:** The data layer (API and Engine) stores exactly what is provided, even if it is logically
   contradictory.
2. **Specific > General:** Specific data provided for a child place always overrides general data from a parent.
3. **Audited Logic:** Complex logical validation is delegated to a specialized the LLM Auditor service rather than being
   hardcoded into the storage layer.

## 2. Inheritance and Overrides

InWheel supports a hierarchical "Parent-Child" relationship (e.g., a Mall containing multiple Shops).

### Top-Down Inheritance

When querying a child place (e.g., a Shop), the system calculates an **Effective Profile** by merging its own
accessibility data with its parent's data.

- The child inherits any accessibility components that the parent has but the child does not (e.g., a shop inherits the
  mall's parking information).
- Inherited components are marked with `is_inherited: true` and include the `source_id` of the parent.

### Specific Overrides

If a child place provides its own data for a specific component type (e.g., its own Entrance status), it **entirely
ignores** the parent's data for that same component type.

- Example: If a Mall has an `accessible` entrance but a Shop inside has an `inaccessible` entrance, the Shop's effective
  profile will show only the `inaccessible` entrance. The Mall's entrance data is discarded for that shop's view.

### No Bottom-Up Propagation

A child's accessibility status or components **never** change the parent's record in the database. Each place maintains
its own source of truth.

## 3. Component vs. Overall Status

The storage and merging layer (`a11y.Engine`) does not perform automatic validation:

- **Component Status:** Technical details (like `step_height: 0.20m`) do not automatically change the component's
  `overall_status`. If a user marks an entrance as `accessible` but provides a 20cm step, the system saves it exactly
  as-is.
- **Place Status:** The statuses of individual components (Entrance, Restroom, etc.) do not automatically update the
  `overall_status` of the entire Place.

This decoupling allows the system to remain a flexible storage engine that preserves the raw input from various
sources (OSM, manual updates, ingestion).

## 4. The Role of the LLM Auditor

Since the data layer allows contradictions, the **LLM Auditor** is responsible for identifying and flagging them.

The Auditor is a background worker that:

1. **Detects Technical Contradictions:** It flags cases where technical properties (e.g., `step_height > 0.05m`)
   contradict a component's `overall_status` (e.g., `accessible`).
2. **Detects Profile Contradictions:** It flags cases where a critical component (like an `inaccessible` entrance) makes
   a profile's `overall_status` (e.g., `accessible`) impossible.
3. **Flags for Review:** Instead of automatically changing the data, it sets `audit.has_conflict = true` and provides
   reasoning. This allows UI layers to show warnings or human moderators to review the data.

By delegating this to the Auditor, the system can handle complex, edge-case-heavy accessibility rules without bloating
the core API logic.
