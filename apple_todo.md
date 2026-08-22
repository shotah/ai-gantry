# Apple stack MCP — hunt

Attack this file. Harness loop work stays in [todo.md](todo.md).
Siblings: [ms_todo.md](ms_todo.md) · [aws_todo.md](aws_todo.md) ·
[gcp_todo.md](gcp_todo.md).

Prefix enable is why a fat iCloud catalog can sit **off** until
`apple__reminders`. That does **not** lift fit gate 3 — `npx` / `uvx` /
Python stay rejected. The list is “what daily question, what API,
write-or-import a static binary (Go / C / C++ / Rust / Swift).” Remote
hosted MCP also waits on **Outbound HTTP MCP** in `todo.md`.

---

**Need: APPLE.** Reminders / Mail / Calendar for people who are not
on Gmail. Google Workspace is the Google-native twin.

**Host split — do not mix these up.** Linux / Docker cannot see
EventKit. macOS native gantry can.

| Path | How | Where it runs |
| --- | --- | --- |
| iCloud protocols | CalDAV / CardDAV / IMAP + app-specific password | Linux, Docker, anything |
| Local Apple apps | EventKit, ScriptingBridge, AppleScript, HealthKit | macOS only |

| Job | Apple app | Packages to look at | Runtime | Notes |
| --- | --- | --- | --- | --- |
| Tasks | **Reminders** | [FradSer/mcp-server-apple-events](https://github.com/FradSer/mcp-server-apple-events), [l22-io/orchard-mcp](https://github.com/l22-io/orchard-mcp), [BRO3886/rem](https://github.com/BRO3886/rem) + [go-eventkit](https://github.com/BRO3886/go-eventkit) | Node+Swift, Swift, Go+cgo | This is the Apple Tasks. Due dates / lists / complete. |
| Calendar | Calendar.app | same EventKit servers; [thetaroot/apple-mcp](https://github.com/thetaroot/apple-mcp) CalDAV | TS / Swift | Linux path = CalDAV. |
| Mail | Mail.app | orchard, [JonathanRReed/Apple-MCPs](https://github.com/JonathanRReed/Apple-MCPs), thetaroot IMAP | TS / Swift | Linux path = `imap.mail.me.com`. |
| Contacts | Contacts.app | orchard; CardDAV (`contacts.icloud.com`) | | |
| Notes | Notes.app | orchard, AppleNotes-MCP | | macOS; AppleScript-limited. |
| Shortcuts | Shortcuts.app | AppleShortcuts-MCP | | macOS; “run this shortcut.” |
| Health | Health.app | [RyanLisse/Vitalink](https://github.com/RyanLisse/Vitalink) | Swift, must be signed | macOS hardware. Skip if Garmin already covers sleep/weight. |
| Messages | Messages.app | AppleMessages-MCP | | macOS. Sending is a second ACL — do not start here. |
| Maps | Maps.app | AppleMaps-MCP | | Skip unless someone refuses the existing `maps__*` MCP. |
| Home | Home.app | almost nothing honest | | Home Assistant is the later HTTP-MCP item, not HomeKit-from-Linux. |
| Photos / Music / Find My / iCloud Drive | | mostly no public API | | Find My and Drive are the usual “can’t.” |

First cut: **Reminders + Calendar + Mail** over iCloud protocols
(works on Linux / Docker). EventKit
is a second binary for macOS-native installs. Do not ship a 65-tool
orchard kitchen sink — prefix `apple__reminders` / `apple__calendar`
/ `apple__mail` like `google__*`.

When something ships: MCP page + `mcp.toml` snippet, then delete the
row here.
