# CSGClaw

> Your Personal AI Team

CSGClaw is a multi-agent collaboration platform built by OpenCSG.

It is not trying to make a single agent do everything. The real problem it tackles is more practical:

**once work becomes non-trivial, how do you get a group of AI agents to operate like a team, without making the system heavy, fragile, or painful to start?**

---

## Why CSGClaw Exists

A single agent can already be useful. But in real projects, the limits show up quickly.

You start to see the same pattern over and over:

- One agent is expected to do everything: coding, research, documentation, testing, coordination.
- Multi-agent workflows sound good in theory, but in practice they often require too much manual setup and supervision.
- Before you can get real work done, you are asked to configure infrastructure, containers, channels, and runtime details.
- You want isolation and safety, but you do not want the operational weight that usually comes with them.
- You want a browser-based experience out of the box, while still keeping the option to connect other channels later.

CSGClaw is built around a simple product goal:

**make multi-agent collaboration feel like a usable product, not a system users have to assemble and maintain themselves.**

---

## What CSGClaw Is

CSGClaw is an AI coordination layer for individuals and small teams.

It gives you one Manager and a set of specialized Workers, so you are no longer juggling several isolated agents by hand. Instead, you work through a single coordination point for:

- defining goals
- breaking work down
- assigning roles
- tracking progress
- collecting results

The experience is closer to working with an AI team than operating a collection of chat windows.

```text
┌──────────────────────────────────────────────────────┐
│                      CSGClaw                         │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │ Manager                                        │  │
│  │ understands goals, plans work, coordinates     │  │
│  └────────────────────────────────────────────────┘  │
│             ↓                    ↓                  │
│      Worker Alice          Worker Bob               │
│        frontend               backend               │
│                                                      │
│      WebUI / Feishu / WeChat / other channels       │
└──────────────────────────────────────────────────────┘
                    ↑
               you make decisions
```

---

## Design Principles

CSGClaw is inspired by the same class of problems that systems like HiClaw try to solve, but it makes different tradeoffs.

If there is one sentence that captures the product direction, it is this:

**lightweight, quick to start, and usable by default.**

### 1. One-click install, minimal setup

CSGClaw is designed to reduce the distance between installation and actual use.

That means:

- fewer steps to get started
- more sensible defaults
- a shorter startup path

The goal is not to force users to understand the whole stack before they can try the product.

### 2. A lighter Manager built on PicoClaw

The Manager in CSGClaw is built on **PicoClaw**.

This is a deliberate product decision:

- lower resource usage
- faster startup
- a better fit for local machines and day-to-day usage

For individuals and small teams, a lighter control layer matters more than adding weight for its own sake.

### 3. A lighter sandbox built on Boxlite

Isolation matters, but many local-first systems make it too expensive from an operational point of view.

CSGClaw uses **Boxlite** as its sandbox foundation so that it can:

- provide isolation without requiring users to install and manage Docker
- keep security boundaries as a default capability
- stay lighter and easier to run locally

This is not just an implementation detail. It reflects a specific product choice: safety should not come bundled with unnecessary setup burden.

### 4. WebUI by default, not tied to a single channel

CSGClaw does not assume one messaging protocol should be the center of the entire system.

Out of the box, it comes with a WebUI. It can also integrate with other channels such as:

- Feishu
- WeChat
- Matrix
- other extensible messaging entry points

That makes CSGClaw feel like a platform, rather than something tightly coupled to one communication layer.

---

## How It Works

The architecture is intentionally straightforward. The important part is role separation.

### Manager

The Manager is responsible for:

- receiving your goals
- deciding whether work should be decomposed
- selecting the right Workers
- tracking execution progress
- consolidating and reporting results back to you

It acts as the coordination layer, not as a giant all-purpose agent that tries to do everything itself.

### Workers

Workers handle execution.

Different Workers can be specialized for different responsibilities, such as:

- frontend development
- backend development
- testing and validation
- documentation
- research

That specialization keeps context cleaner, reduces role confusion, and makes collaboration easier to manage.

### Sandbox

Worker execution is isolated through Boxlite.

This gives CSGClaw a more practical balance:

- users do not need Docker just to get started
- the system does not have to run without meaningful isolation

### Interface Layer

The default user-facing interface is the built-in WebUI, while other channels remain available as integrations.

The point is not complexity. The point is flexibility:

- use it immediately in the browser
- connect team communication tools when needed
- avoid locking the platform into a single entry point

---

## A Typical Workflow

A concrete example explains the model better than abstract architecture diagrams:

```text
You: Build a simple web app prototype with a landing page, login page, and a basic admin view.

Manager: Understood. I am splitting this into tasks.
  - Alice owns the landing page and login page
  - Bob owns the backend APIs and data model
  - Carol handles integration checks and validation

Alice: Starting page structure and styling
Bob: Defining APIs and mock data

Manager: Frontend and backend are now moving in parallel. Waiting for the first integration-ready version.

You: Add GitHub login to the login flow.

Manager: Noted. Updating Alice and Bob.

Carol: First integration pass found that the login response is missing the user avatar field.

Manager: Issue recorded. Bob will update the API first, then Alice will update the UI once the field contract is confirmed.
```

The important part is not simply that multiple agents exist.

The important part is that **their collaboration is organized**.

---

## Why It Fits Real-World Use Better

Many multi-agent systems make sense conceptually, but break down in day-to-day usage for two common reasons:

- they are too heavy to start
- they are too fragmented to operate comfortably

CSGClaw is designed to avoid both.

### Lighter

- the Manager is built on PicoClaw
- the sandbox is built on Boxlite
- the default entry point is the built-in WebUI

### Faster

- a shorter startup path
- fewer local dependencies
- better suited to always-on, day-to-day use

### More natural to use

- you work through one coordination layer
- Workers can stay role-specific
- channels can be added based on real workflow needs

### More pragmatic

CSGClaw does not assume users already have a full infrastructure stack. It does not assume they want to maintain Docker. It does not assume everyone wants to center their workflow around one messaging protocol.

It starts from a simpler question:

**can people actually use this today without turning setup into a project of its own?**

---

## Who It Is For

CSGClaw is a strong fit for:

- independent developers who want an AI team instead of a single assistant
- small teams that want lower-friction multi-agent collaboration
- users who care about startup speed, lighter runtime cost, and sensible defaults
- people who want to start from a WebUI instead of binding the whole system to Matrix or another single channel

---

## Product Direction

CSGClaw is not trying to become a more complicated agent platform.

It is trying to become a more usable AI team platform.

"Your Personal AI Team" is not just a slogan. It implies a concrete product standard:

- installation should be simple
- startup should be fast
- collaboration should be clear
- isolation should be practical
- interfaces should stay flexible

If a multi-agent system cannot meet those conditions at the same time, it will struggle to become part of everyday work.

That is the direction CSGClaw is built around.

---

## Acknowledgement

CSGClaw is informed by ideas explored in HiClaw, especially around the usability of multi-agent collaboration.  
At the implementation level, however, CSGClaw places stronger emphasis on lightweight runtime choices, easier local startup, and a platform model that is not tightly bound to a single communication channel.
