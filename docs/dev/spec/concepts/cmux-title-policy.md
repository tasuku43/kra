---
title: "cmux workspace title and ordinal policy"
status: implemented
---

# cmux Workspace Title And Ordinal Policy

## Purpose

Provide deterministic title rendering for cmux workspace creation flows while retaining legacy ordinal metadata
compatibility.

## Title Format

- `"<kra-id> | <kra-title>"`
- The 1:1 mapping policy does not render an ordinal suffix.

## Fallback Rules

- If `kra-title` is empty, use `(untitled)`.
- `kra-id` is required.
- The legacy ordinal argument is accepted for mapping compatibility but does not affect title rendering.

## Legacy Ordinal Metadata

- Schema v1 continues to store 1-based `ordinal` and `next_ordinal` values so existing files remain readable.
- A newly-created mapping normally receives ordinal `1`; stale mapping replacement resets allocation before
  creating the replacement.
- Ordinal metadata is not a user-facing workspace multiplicity contract.
