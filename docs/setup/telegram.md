# Telegram Setup

This is a one-time, operational (not code) setup to get the values `telegram.*` config needs. Skip this entirely if you're only using Slack (`telegram.enabled: false`).

## 1. Create the bot

1. Open a chat with [@BotFather](https://t.me/BotFather) on Telegram.
2. Send `/newbot` and follow the prompts (choose a name and a username ending in `bot`).
3. BotFather replies with a token like `123456789:AAF...` — this is `telegram.bot_token` / `APP_TELEGRAM_BOT_TOKEN`.

## 2. Add the bot to where you want notifications

- **Group chat**: create or open the group, add the bot as a member (a regular member is enough — Hedwig only sends messages, it doesn't need admin rights unless you want it to post in a channel, see below).
- **Channel**: add the bot as an **administrator** of the channel (channels require posting bots to be admins).

## 3. Get the chat ID

Hedwig needs the numeric ID of the chat/channel to post into — this is `telegram.chat_id`, and it's *not* the same as the `@username`.

1. Send any message in the group/channel (mentioning the bot isn't required).
2. Call `https://api.telegram.org/bot<your-bot-token>/getUpdates` in a browser or via `curl`, and look for `"chat":{"id": ...}` in the JSON response near the message you just sent.
3. Group and channel IDs are negative numbers, often starting with `-100` (e.g. `-100123456789` in `config.example.yaml`) — a plain private chat ID is a smaller positive number, but Hedwig is designed to post into a single shared group/channel, not a DM.

If `getUpdates` returns an empty `result`, send another message after adding the bot (Telegram only queues updates from after the bot joined) and retry.

## 4. Pick a webhook secret and URL

- `telegram.webhook_secret` / `APP_TELEGRAM_WEBHOOK_SECRET`: any random value you choose (e.g. `openssl rand -hex 32`). Telegram echoes this back as the `X-Telegram-Bot-Api-Secret-Token` header on every webhook delivery, and Hedwig rejects requests where it doesn't match.
- `telegram.webhook_url`: the public HTTPS URL Telegram should call — your domain plus `telegram.webhook_path` (default `/webhooks/telegram`), e.g. `https://hedwig.example.com/webhooks/telegram`.

You do **not** need to call Telegram's `setWebhook` API yourself — Hedwig does this automatically at startup (`cmd/bot/main.go`, using exactly the `webhook_url`/`webhook_secret` from config), each time it starts.

## Result

You should now have four values for the `telegram:` section of `config.yaml`:

```yaml
telegram:
  enabled: true
  bot_token: ""         # set via APP_TELEGRAM_BOT_TOKEN, step 1
  webhook_secret: ""    # set via APP_TELEGRAM_WEBHOOK_SECRET, step 4 (any random value)
  webhook_path: /webhooks/telegram
  webhook_url: https://hedwig.example.com/webhooks/telegram   # step 4
  chat_id: -100123456789   # step 3
```
