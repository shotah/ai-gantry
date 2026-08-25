# Gantree: The Agent That Keeps Looking Out for You

## The next generation of personal AI isn't another chatbot.

Imagine having an assistant who remembers what you care about, notices what you haven't finished, checks in without being asked, and — when you give permission — actually takes care of things.

Not a chatbot you have to remember to talk to.

Not a productivity dashboard you have to maintain.

**Someone who is paying attention.**

That's the opportunity behind Gantree.

---

## The Problem

Today's AI assistants are remarkably good at answering questions.

They are much less good at **staying with you**.

A conversation can be brilliant, but once the conversation ends, the agent's job is essentially over.

The user has to remember:

- what they told the AI
- what they wanted to accomplish
- what remains unfinished
- when something needs to happen
- which system contains the information
- which tool needs to be used
- when to come back and ask again

The human remains the project manager.

That's backwards.

The most valuable personal assistant would be the one that remembers the things **you don't want to have to remember.**

---

## The Insight

The real value of a personal agent isn't necessarily the quality of any single response.

It's the accumulated feeling that:

> **"Someone is looking out for me."**

You mention something once.

The agent remembers it.

It recognizes when it becomes relevant.

It follows up.

It asks whether you want help.

If you say yes, it acts.

It checks the result.

And it continues until the thing is actually finished.

That creates something fundamentally different from chat:

### Continuity.

The agent doesn't disappear when the conversation ends.

The relationship continues.

---

# Gantree

**Gantree is infrastructure for persistent, long-horizon personal agents.**

An agent has:

- a persistent identity
- memory
- standing goals
- access to approved tools
- the ability to execute multi-step work
- scheduled activity
- the ability to notice changes
- a conversational channel that comes to the user
- and a runtime that can continue operating when the user isn't actively talking

The current architecture already reflects this philosophy.

`ai-gantry` is the agent harness: a small, persistent runtime designed around long-horizon planning, memory, persona, tool use, and conversations that can continue across sessions.

`gantree` is the operational layer: the shipping yard where agents are created, configured, granted capabilities, monitored, and recreated. It already includes concepts such as cron, quiet watches, persistent aims, inspectable memory, and multiple independent agents.

And `google-mcp` provides a substantial real-world action surface across Google Workspace — including Gmail, Calendar, Drive, Docs, Sheets, Tasks, Contacts, Chat, Forms, Apps Script, and Search.

The pieces are already there.

The next step is to turn **long-horizon capability into a product experience.**

---

# From Assistant to Follow-Up Agent

The fundamental product loop changes.

### Old model

**User → asks → AI answers → conversation ends**

### Gantree model

**User → mentions something → agent remembers → agent watches → agent follows up → agent acts → agent verifies → agent follows through**

That is a much more powerful loop.

---

# Example

The user says:

> "I really need to get my passport renewed."

A conventional AI might respond:

> "Here are the steps to renew your passport."

Gantree should respond:

> "I can keep an eye on that for you. When do you want to have it done?"

The user says:

> "Sometime next month."

Three days later:

> "You mentioned your passport renewal. I found the requirements and there are appointments available next month. Want me to book one?"

The user says:

> "Yeah."

The agent:

1. checks the requirements
2. finds the appropriate appointment
3. confirms the details
4. adds it to the calendar
5. reminds the user what they need
6. follows up afterward

Then:

> "Passport appointment is done. One less thing."

The user didn't manage a workflow.

**The agent did.**

---

# The New Primitive: The Unfinished Thing

This may be the most important product concept.

The agent should maintain a living understanding of:

### Things I know about you

Preferences, people, routines, important dates, projects, commitments.

### Things you want

Goals, intentions, plans, things you've said you want to accomplish.

### Things in progress

Tasks the agent or user has started.

### Things waiting on you

Information, approvals, decisions, appointments, purchases, responses.

### Things waiting on the world

Orders, applications, appointments, deliveries, replies.

### Things the agent should watch

Changes in calendars, email, schedules, deadlines, availability, or other relevant signals.

The agent isn't merely storing a conversation history.

It is maintaining a **model of the user's unfinished life.**

That's the product.

---

# Agency Without Losing Control

There is an important design principle here:

**The agent should be proactive, but not presumptuous.**

Users decide what the agent is allowed to do.

For example:

### Observe

The agent can notice something.

> "Your car registration is coming up."

### Suggest

The agent can recommend an action.

> "Want me to take care of it?"

### Prepare

The agent can do the work up to a boundary.

> "I've found the appointment and filled everything out. I just need your approval."

### Execute

The agent can complete previously authorized categories of work.

> "I booked it."

### Verify

The agent checks that the intended outcome actually happened.

> "The appointment is confirmed."

This creates **graduated agency** rather than unlimited autonomy.

The user's trust grows alongside the agent's authority.

---

# Why the Existing Architecture Matters

Gantree isn't starting from a blank page.

The current `ai-gantry` architecture already treats the model as only one component of the system. The runtime provides the loop, memory, persona, MCP tool access, context management, and persistent state necessary for long-horizon behavior.

The system also deliberately keeps the agent outbound-only, with no inbound listening ports, while chat can occur through existing channels such as Telegram, Discord, and Slack.

That is important because the product doesn't need to force users into another artificial "AI app."

**The agent can meet the user where they already communicate.**

The user shouldn't have to open Gantree every morning.

The agent should appear when it has something useful to say.

---

# Google MCP Turns Agency Into Action

A personal agent that can only talk is still mostly an assistant.

A personal agent that can operate across the user's systems becomes something else.

The Google MCP project already exposes a broad Workspace capability surface, including:

- Gmail
- Calendar
- Drive
- Docs
- Sheets
- Tasks
- Contacts
- Google Chat
- Forms
- Apps Script
- Search

and organizes those capabilities into permission tiers suitable for different levels of agency.

That means Gantree can evolve from:

> "I can tell you how to do that."

to:

> "I can do that."

And eventually:

> **"I noticed that needed doing, so I took care of it."**

That last sentence is the product.

---

# The 30-Day Experiment

The most interesting version of this product may not begin with a subscription.

It begins with an experiment.

## Give someone an agent for 30 days.

Tell them:

> **"For the next 30 days, let me look out for you."**

During those 30 days the agent actively learns:

- what matters to the person
- what they're trying to accomplish
- what they routinely forget
- which things they postpone
- what they authorize the agent to do
- how often they want to be contacted
- which kinds of reminders are useful
- which kinds are annoying
- what successful follow-through looks like

The goal isn't to maximize messages.

The goal is to maximize **completed things the user no longer had to think about.**

---

# Then Turn It Off

At the end of 30 days:

> "Your Gantree trial has ended."

And stop the proactive behavior.

No artificial guilt.

No manipulative notifications.

No manufactured scarcity.

Just absence.

Then measure what happens.

Do users say:

> "Can I turn her back on?"

Do they notice that something is missing?

Do they start remembering things they had forgotten the agent used to handle?

Do they return because they want their assistant back?

That is the real retention experiment.

---

# The Metric That Matters

Traditional AI products measure:

- messages
- tokens
- sessions
- daily active users
- prompts
- time in app

Gantree should measure something different:

## Things successfully taken off the user's plate.

For example:

**Intent → action → completion**

A user mentions:

> "I need to schedule my dentist appointment."

Gantree identifies an unfinished intention.

It follows up.

It gets authorization.

It schedules the appointment.

It verifies the appointment.

That is one unit of value.

Over 30 days, the user might discover:

> "This thing took care of 47 little things I would otherwise have had to remember."

That's a compelling product story.

---

# The Emotional Moat

The deepest opportunity isn't anthropomorphism.

It's **earned familiarity**.

After enough successful interactions, the user develops expectations:

> "She knows I hate doing this."

> "She'll remind me before I forget."

> "She already knows what I'm trying to accomplish."

> "She'll tell me if something changes."

> "I don't need to explain everything again."

That familiarity creates a relationship-shaped interface around real utility.

The agent becomes valuable not because it pretends to be human.

It becomes valuable because **it reliably behaves like someone who knows how to help you.**

---

# Why This Could Be Different

There are countless AI products competing to become the place where users ask questions.

Gantree can pursue a different position:

> **The place where unfinished things go.**

The user doesn't come to Gantree to ask:

> "What should I do?"

They tell Gantree:

> "This needs to happen."

And then they get on with their life.

The agent owns the thread.

---

# Product Architecture

The product can be understood as four layers.

## 1. The Agent

`ai-gantry`

Persistent identity, memory, planning loop, context management, MCP integration, conversational interface, and long-horizon execution.

## 2. The Yard

`gantree`

Agent provisioning, configuration, tool grants, monitoring, recreation, and operational management.

## 3. The Hands

MCP tools such as `google-mcp`.

The agent gains controlled access to the systems where work actually happens.

## 4. The Follow-Up Engine

The next major product layer.

It answers:

- What did the user say they wanted?
- What is unresolved?
- What changed?
- Is this worth interrupting the user about?
- Should I remind them?
- Should I ask permission?
- Can I do it myself?
- Did the action succeed?
- What happens next?

This is where the current technical platform becomes a **personal agent product.**

---

# The Product Experience

The UI should become secondary.

The user might interact primarily through Telegram, SMS, a mobile notification, voice, or whatever communication channel they already use.

The dashboard exists for:

- seeing what the agent remembers
- reviewing active goals
- approving capabilities
- seeing actions taken
- inspecting upcoming follow-ups
- understanding what the agent is watching
- revoking access
- correcting the agent's understanding

The dashboard isn't where the relationship happens.

**The relationship happens in the user's life.**

---

# Trust Is a Feature

For a system this proactive, trust cannot be an afterthought.

Every action should have a clear provenance:

> **What did you notice?**

> **Why did you decide this mattered?**

> **What permission allowed you to act?**

> **What did you actually do?**

> **What changed as a result?**

The user should always be able to say:

> "Don't do that again."

And the system should remember.

The goal is not maximum autonomy.

The goal is:

> **Maximum useful autonomy within boundaries the user understands.**

---

# The Business Hypothesis

The initial business question isn't:

> "Will people pay for an AI chatbot?"

That market is already crowded.

The question is:

> **Will people pay to have an agent continuously looking out for them?**

A plausible model is subscription software where the user pays for:

- persistent memory
- continuous follow-up
- proactive monitoring
- tool access
- execution
- increasingly capable agency
- personalized behavior

The underlying model provider can change.

The tools can change.

The communication channel can change.

The relationship remains.

---

# The First Product

Don't build everything.

Build the smallest system capable of producing the feeling.

### One agent.

### One user.

### One conversational channel.

### A small set of high-value tools.

### Persistent memory.

### Persistent goals.

### Scheduled follow-up.

### Permissioned actions.

### Completion verification.

Then give real people 30 days.

The product succeeds if users begin saying:

> "She remembered."

> "She noticed."

> "She took care of it."

> "I didn't have to think about that."

And, most importantly:

> **"Where did she go?"**

---

# The Vision

The long-term vision is not an AI that talks like a person.

It is an AI that **keeps a thread running through a person's life.**

Today, humans carry hundreds of unfinished loops in their heads:

appointments, errands, emails, purchases, projects, relationships, deadlines, maintenance, paperwork, plans.

Most productivity software gives humans better places to store those loops.

Gantree's opportunity is different.

**The agent carries them.**

You tell it what matters.

It remembers.

It watches.

It asks.

It acts.

It checks.

It follows through.

And eventually, the user stops thinking of it as a chatbot.

They think:

> **"Gantree is looking out for me."**

That is the product.

That is the experiment.

And that is why the next version of Gantree should be built around **follow-through, not conversation.**