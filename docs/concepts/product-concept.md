---
title: "Product Concept: AI-Assisted Knowledge Work"
status: implemented
date: 2026-02-14
---

# Product Concept

## 1. Purpose

This document defines the core product concept for `kra`: the problem it solves,
who it is for, and the value it should deliver.

## 2. Problem

Modern knowledge work with AI assistants generates large output across coding,
investigation, incident response, and definition tasks. Speed increases, but
traceability often degrades.

As a result, workers struggle to:

1. Resume work quickly after interruptions
2. Explain what was done and why
3. Reuse past findings effectively
4. Turn day-to-day work into reviewable outcomes

## 3. Primary Persona

**AI-Assisted Knowledge Worker**

- Works across implementation and non-implementation tasks
- Uses external ticket systems (for example Jira) as task source-of-truth
- Uses AI assistants heavily in day-to-day execution
- Needs outputs that stay revisitable and explainable later

## 4. Operating Context

- Task management stays outside `kra`
- `kra` is a local execution and traceability layer
- Main interaction is CLI + AI assistants
- Work themes change frequently and are not fixed by job type

## 5. Core Value Proposition

`kra` should help users execute diverse AI-assisted work quickly while keeping
high traceability, without replacing existing external task systems.

## 6. Non-goals

1. Replacing Jira or other external task-management tools
2. Managing complex dependency graphs inside `kra`
3. Acting as a GUI planning/whiteboard replacement
