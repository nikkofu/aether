# Architecture

## Core Principle

LLMs do not directly control execution.

All actions pass through:

Decision → Policy → Execution

---

## Everything Is a DAG Node

Even a single CLI call is a task node.

---

## Multi-Agent First

Designed for parallel AI cooperation.

---

## Replaceable Adapters

Gemini, Claude, OpenAI, etc.

Must implement a unified interface.
