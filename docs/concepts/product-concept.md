---
title: "Product Concept: AI-Agent Driven Knowledge Work"
status: proposed
date: 2026-02-14
---

# Product Concept

## 1. Purpose

This document defines the core product concept for `kra`: the problem it solves,
who it is for, and the value it should deliver.

## 2. Problem

Modern knowledge work with AI agents generates a large volume of outputs across
many work types: implementation, investigation, incident response, metrics analysis,
and concept definition. Teams move quickly, but traceability often breaks down.

As a result, workers struggle to:

1. Resume work quickly after interruptions
2. Explain what was done and why
3. Reuse past findings effectively
4. Turn day-to-day work into reviewable outcomes

## 3. Who It Is For (Primary Persona)

### Persona Name

**AI-Agent Driven Knowledge Worker**

### Profile

- Cross-functional practitioner working across implementation, investigation,
  incident response, and definition work
- Uses external ticketing systems (e.g., Jira) as the system of record for task management
- Uses AI agents heavily in day-to-day execution
- Produces high-volume outputs that must be revisitable and explainable later

### Behavioral Traits

- Switches between coding and non-coding work multiple times per day
- Pulls evidence from multiple sources (code, tickets, metrics, logs, docs)
- Needs periodic narrative output for review, reporting, and career records

### Success Criteria

1. Can start work quickly from an external ticket context
2. Can recover context quickly after interruption
3. Can explain what was done and why afterward
4. Can convert work traces into outcomes for evaluation and reporting

## 4. Operating Context

- Task-management source of truth lives outside `kra`
- `kra` is a local execution and traceability layer
- Work is primarily CLI and AI-agent driven
- Work themes change frequently and are not fixed by job type

## 5. Core Value Proposition

`kra` should help users execute diverse AI-assisted work with speed and
maintain high traceability without replacing external ticket tools.

## 6. Non-goals

1. Replacing Jira or other external task-management tools
2. Managing complex task dependency graphs inside `kra`
3. Acting as a GUI whiteboard or visual planning replacement
