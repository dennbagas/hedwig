# Hedwig — GitHub-Telegram Notification & PR Bot

A service connecting GitHub and Telegram for two purposes:
 
1. **Notifications** — relay GitHub repository events (push, PR opened/closed, CI/CD status) to Telegram, with a retry action for failed CI/CD runs.
2. **PR creation bot** — a Telegram-driven, multi-step flow to open a pull request between a chosen source and target branch, for a chosen repository.
Both features share one GitHub App identity, one Telegram bot, and one persistence layer.
