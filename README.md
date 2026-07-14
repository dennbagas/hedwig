# Hedwig — GitHub-Telegram Notification Bot

A service connecting GitHub and Telegram: relays GitHub repository events
(push, PR opened/closed, CI/CD status) to Telegram, with a retry action for
failed CI/CD runs. Uses one GitHub App identity, one Telegram bot, and one
persistence layer.

See [`docs/PRD.md`](docs/PRD.md) for the full phased roadmap — this is
phase 1. Phase 2 (a bulk PR-creation feature driven by a Google Doc
deployment checklist) is planned — see
[`docs/phase-2-pr-creation.md`](docs/phase-2-pr-creation.md).
