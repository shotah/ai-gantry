# AI-Gantry — Rosie Follow-Through Feature Specification

## Objective

Turn the existing AI-Gantry agent into a genuinely persistent follow-through agent.

Do **not** create additional agents, orchestration layers, specialized follow-up services, or a new workflow framework.

The existing agent is the intelligence.

The goal is to give that agent a better ability to:

**notice → remember → schedule → wake → act → follow up**

Keep the implementation small and reuse existing AI-Gantry primitives wherever possible.

---

## 1. Audit Existing Capabilities First

Before changing code, inspect the existing AI-Gantry implementation and identify:

- How an agent is identified and persisted.
- How memory is created.
- How memory is retrieved.
- How cron jobs are created.
- How cron jobs are executed.
- How a cron invocation reaches the agent.
- How the agent loads its persona/system instructions.
- How the agent sends outbound messages.
- How MCP tools are exposed.
- How an agent persists state between invocations.

### Requirement

Do not implement a new mechanism for something AI-Gantry already supports.

The first goal is to discover how close the current implementation already is to supporting follow-through.

---

# 2. Add Follow-Through Behavior to the Existing Persona

Update the existing persona template.

**Do not create multiple persona files.**

Do not create a separate Follow-Up Agent, Reminder Agent, Planning Agent, or similar abstraction.

The existing persona should teach the existing agent that it has responsibility for things that happen after the current conversation.

The persona should instruct the agent to recognize:

- commitments
- intentions
- future events
- deadlines
- things the user wants remembered
- things the user is waiting for
- things the agent should check later
- tasks that may benefit from proactive assistance

The agent should consider whether something deserves persistent memory and/or a future follow-up.

---

# 3. Teach the Agent to Schedule Its Own Follow-Ups

The agent should be explicitly encouraged to use the existing cron mechanism when appropriate.

Examples:

> "Remind me tomorrow."

→ remember the request and create a cron.

> "My appointment is next Thursday."

→ remember the appointment and consider an appropriate follow-up.

> "I need to renew my passport next month."

→ remember the intention and schedule a useful future check.

> "I'm waiting for John to get back to me."

→ remember the waiting state and schedule a future check.

The agent should be able to decide that a follow-up is useful without the user explicitly saying "remind me."

---

# 4. Follow-Up Memory Must Contain Context

When the agent schedules future work, it must leave enough persistent memory for its future self to understand the reason for the cron.

A future invocation should not depend on the original conversation still being in the active context window.

A useful follow-up should allow future Rosie to answer:

- What was the user trying to accomplish?
- Why did I schedule this?
- When did this originate?
- What was supposed to happen?
- What has already happened?
- What should I check or do now?

Use the existing memory system.

Do not create a second follow-up database unless the existing architecture genuinely requires one.

---

# 5. Improve Cron → Agent Context

When a cron fires, the existing agent should receive enough information to understand that this is a scheduled continuation of previous work.

The invocation should effectively communicate:

> You scheduled this future work yourself. Retrieve the relevant memory/context and determine what should happen now.

The agent should then reason normally.

Do not create a specialized follow-up executor.

**The normal agent is the executor.**

---

# 6. Follow-Up Does Not Mean "Send a Reminder"

When a scheduled follow-up fires, Rosie should decide what the most useful action is.

Possible outcomes include:

- send the user a reminder
- ask the user a question
- perform work using an available tool
- check an external system
- complete the task
- determine that no action is needed
- postpone the follow-up
- cancel the follow-up
- create another follow-up

The important behavior is:

> **A cron wakes Rosie up. Rosie decides what to do.**

Do not hard-code a workflow for each type of follow-up.

---

# 7. Allow Follow-Ups to Continue Themselves

A follow-up should not necessarily be a one-shot event.

Example:

```text
User mentions passport renewal
        ↓
Rosie remembers it
        ↓
Rosie schedules follow-up
        ↓
Rosie wakes up
        ↓
Rosie checks what is needed
        ↓
Rosie contacts user
        ↓
User says "yes, handle it"
        ↓
Rosie performs the action
        ↓
Rosie schedules verification
        ↓
Rosie wakes up again
        ↓
Rosie verifies completion
```

The agent should be able to create the next cron as part of normal reasoning.

This is the core long-horizon behavior.

---

# 8. Support Relative and Absolute Dates

Ensure the agent can create useful follow-ups for:

### Absolute dates

- August 30
- October 12 at 9 AM
- next Thursday at 3 PM

### Relative dates

- tomorrow
- next week
- in three days
- two hours from now
- one day before the event

### Event-relative follow-ups

Examples:

> "Remind me one day before my appointment."

> "Check whether John responded two days after I email him."

> "Follow up a week after I submit the application."

Use the existing cron implementation wherever possible.

---

# 9. Support Event Memory

Teach Rosie to recognize meaningful future events.

Examples:

- birthdays
- appointments
- travel
- deadlines
- renewals
- meetings
- reservations
- bills
- applications
- deliveries
- personal commitments

The event itself should be remembered.

Follow-up cron jobs can then be created around the event.

Example:

```text
Memory:
Mom's birthday is October 12.

Cron:
October 5 → think about gift

Cron:
October 11 → remind user / help with final preparation
```

Do not build a separate calendar subsystem for this.

---

# 10. Support "Waiting For" Behavior

Rosie should recognize when something is unresolved because another person/system has not responded.

Examples:

> "I'm waiting for John to respond."

> "I submitted the application and haven't heard back."

> "The package should arrive Friday."

The agent should be able to:

1. remember the waiting state
2. schedule a future check
3. wake up
4. inspect available information
5. determine whether anything changed
6. act or communicate
7. schedule another check if necessary

This creates continuity without requiring the user to remember to ask again.

---

# 11. Completion and Cancellation

Rosie should be able to recognize when a follow-up is no longer needed.

Examples:

> User: "Never mind, I already did it."

> User: "John got back to me."

> User: "Cancel that."

The agent should cancel or stop future work using existing mechanisms.

Avoid leaving stale cron jobs behind.

---

# 12. Prevent Duplicate Follow-Ups

Before creating a new follow-up, Rosie should check whether the same thing is already being tracked.

Avoid situations like:

```text
Passport renewal reminder
Passport renewal reminder
Passport renewal reminder
Passport renewal reminder
```

The simplest solution is preferable:

- retrieve relevant memory
- check existing scheduled work
- avoid creating a duplicate

Do not build a sophisticated deduplication service.

---

# 13. Respect User Preference and Quiet Hours

Use existing agent/user configuration where available.

Rosie should understand that proactive behavior is useful only when it remains useful.

The persona should encourage:

- don't nag
- don't repeat something unnecessarily
- don't interrupt without a reason
- don't manufacture urgency
- don't guilt the user
- prefer useful action over unnecessary messaging

If quiet hours already exist in AI-Gantry, use them.

If they don't, do not build an elaborate notification preference system as part of this feature.

Keep the first implementation simple.

---

# 14. Preserve the Existing Agent Identity

This is important.

A scheduled invocation must be the **same agent**.

It should retain:

- the same agent ID
- the same persona
- the same memory
- the same tool permissions
- the same relationship/context with the user

The experience should be:

> Rosie talked to me yesterday.

and later:

> Rosie remembered what we discussed yesterday.

Not:

> Some new process generated a reminder.

---

# 15. Preserve Existing MCP / Tool Architecture

Do not introduce new tool abstractions unless required.

The agent should use the existing MCP tools exactly as it does during normal conversation.

The important new behavior is simply:

> **Rosie can decide to use those tools during a future invocation without the user initiating the conversation at that moment.**

This is where existing Google MCP capabilities become especially powerful.

For example:

```text
Remember:
"User needs to schedule dentist appointment."

↓

Cron fires

↓

Rosie checks memory

↓

Rosie uses Calendar / Gmail / other approved tools

↓

Rosie does useful work

↓

Rosie contacts user

↓

Rosie schedules next check if necessary
```

---

# 16. Make Scheduled Invocations Idempotent

A cron firing should be safe if the same task has already been completed.

For example:

If Rosie wakes up to check:

> "Did John respond?"

but John already responded, Rosie should recognize that and finish the thread rather than creating more work.

The agent should check current state before acting.

Do not create a complex workflow engine to guarantee this.

Use memory + current tool state + normal reasoning.

---

# 17. Add Logging for Follow-Through

Add enough logging to answer:

- What caused the follow-up?
- Which agent scheduled it?
- When was it scheduled?
- When did it fire?
- What memory/context was associated with it?
- What did Rosie decide to do?
- Did Rosie create another follow-up?
- Was the follow-up completed/cancelled?

This is primarily for development and the 30-day experiment.

Keep it compatible with the existing AI-Gantry logging architecture.

---

# 18. Add a Small Test Suite

Create tests around the core loop.

At minimum:

### Test 1 — Explicit reminder

User:

> "Remind me tomorrow to call the dentist."

Expected:

- memory exists
- cron exists
- cron fires
- agent receives appropriate context

### Test 2 — Implicit intention

User:

> "I need to renew my passport next month."

Expected:

- Rosie recognizes the future obligation
- appropriate memory is created
- appropriate follow-up is scheduled

### Test 3 — Follow-up continuation

Cron fires.

Expected:

- Rosie retrieves the relevant memory
- Rosie reasons about what to do
- Rosie can send a message or perform an action
- Rosie can create another follow-up

### Test 4 — Completion

User indicates the task is complete.

Expected:

- stale future follow-up is not repeated

### Test 5 — Existing follow-up

Same intention is mentioned again.

Expected:

- Rosie does not blindly create duplicate follow-ups.

---

# 19. Do Not Build These Things

Explicitly avoid:

- multiple specialized agents
- a Follow-Up Agent
- a Reminder Agent
- a Planning Agent
- an Orchestrator Agent
- a workflow engine
- a second memory system
- a separate event database
- a separate scheduling service
- multiple persona files
- a new agent hierarchy
- a new "agent protocol"
- complex state machines unless existing code genuinely requires one

The existing agent is the system.

Use:

**persona + memory + cron + MCP + existing runtime**

before adding anything else.

---

# 20. Definition of Done

AI-Gantry is ready for the Rosie experiment when this works end-to-end:

### Conversation

User:

> "My mom's birthday is October 12. I always forget to buy her something."

### Rosie

Recognizes that this is a future obligation.

Stores the relevant memory.

Creates an appropriate future cron.

### Time passes

The cron fires.

### Rosie wakes up

The same Rosie instance receives the scheduled work.

Rosie retrieves the memory.

Rosie understands why the follow-up exists.

Rosie decides what would be useful.

Rosie may:

- message the user
- search for something
- use an MCP tool
- ask a question
- schedule another follow-up

### User responds

Rosie continues normally.

### Eventually

The birthday is handled.

Rosie stops scheduling work related to it.

---

# The Core Principle

Do not make AI-Gantry into a system that manages agents managing agents.

Make it a system where:

> **One persistent agent can remember something today and take responsibility for it tomorrow.**

The agent already has the intelligence.

The implementation should simply give that intelligence a reliable way to **leave itself a note and wake itself back up later.**

That is the feature.