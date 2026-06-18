# Product

## Register

product

## Users

epsilon is for developers running agentic coding sessions from the terminal. They are working inside local codebases, watching an agent reason, inspect files, request permissions, apply edits, and resume previous sessions. Their context is active engineering work, so the TUI needs to make current state, agent activity, tool use, and session controls visible without getting in the way of typing and review.

## Product Purpose

epsilon is an agent harness and terminal TUI for practical coding-agent sessions. It provides a provider-neutral core, persistent sessions, workspace tools, permission brokering, slash commands, model controls, and observability around what the agent is doing. Success means a developer can start, monitor, steer, approve, resume, and understand an agentic coding session from the terminal with confidence.

## Brand Personality

Sharp, observable, delightful. The interface should feel terminal-native and developer-serious, but not austere: polished chat surfaces, meaningful color, clear agent activity, and small moments of craft should make the workflow feel alive. The tone is direct and capable, with personality coming from precision, responsiveness, and thoughtful status detail rather than marketing language.

## Anti-references

epsilon should not look like a generic SaaS dashboard, a decorative chat toy, or an over-branded terminal skin. Avoid hiding agent behavior behind vague loading states, flattening all events into undifferentiated transcript text, or using color as decoration without information value. The TUI should not sacrifice keyboard flow, dense observability, or readable contrast for novelty.

## Design Principles

1. Make agent activity legible: tool calls, permissions, model state, context use, streaming, and session state should be easy to scan while work is happening.
2. Keep the terminal in charge: keyboard-first interaction, compact layouts, fast feedback, and predictable terminal conventions should anchor every design decision.
3. Use color as state, structure, and delight: color should help distinguish roles, risk, activity, and progress while giving the interface a memorable identity.
4. Polish the chat surface: messages, markdown, tool results, diffs, and prompts should feel carefully composed, readable, and stable across terminal sizes.
5. Preserve developer trust: every control should make its consequence clear, especially around permissions, file edits, session resume, model changes, and command execution.

## Accessibility & Inclusion

The TUI should remain keyboard-first and screen-density aware. Text, status colors, and placeholders should maintain strong contrast on the black terminal background. Color should not be the only cue for important states; labels, borders, symbols, or copy should reinforce errors, approvals, selections, and progress. Motion and animated spinners should be restrained and never block comprehension.
