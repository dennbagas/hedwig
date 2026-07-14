# Hedwig — GitHub-Telegram Notification & PR Bot

A service connecting GitHub and Telegram: relays GitHub repository events
(push, PR opened/closed, CI/CD status) to Telegram, with a retry action for
failed CI/CD runs. Uses one GitHub App identity, one Telegram bot, and one
persistence layer.

Delivered in phases:

- **[Phase 1 PRD: Notifications & CI/CD Retry Bot](docs/phase-1-notifications.md)** —
  **shipped**. Relays GitHub repository events to Telegram and offers a retry
  button for failed CI/CD runs.
- **[Phase 2 PRD: PR Creation Bot](docs/phase-2-pr-creation.md)** —
  **redesigned, not yet implemented**. A first Telegram-wizard version was
  built then reconsidered; the current plan is a bulk PR-creation flow
  driven by a Google Doc deployment checklist.

See each linked document for that phase's goals, non-goals, user stories,
functional requirements, and technical design.
