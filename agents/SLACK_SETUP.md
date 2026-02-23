# Slack App Setup Guide for Rune Scribe

Scribe needs a Slack App with Event Subscriptions to receive messages from your workspace.

## Prerequisites

- A Slack workspace where you have admin permissions
- enVector Cloud endpoint and API key
- Python 3.12+ installed

## 1. Create a Slack App

1. Go to https://api.slack.com/apps
2. Click **Create New App** → **From scratch**
3. App Name: `Rune Scribe` (or any name you prefer)
4. Workspace: select your target workspace
5. Click **Create App**

## 2. Configure Bot Token Scopes

Go to **OAuth & Permissions** in the left sidebar, scroll to **Bot Token Scopes**, and add:

| Scope | Purpose |
|-------|---------|
| `channels:history` | Read messages in public channels |
| `groups:history` | Read messages in private channels |
| `im:history` | Read direct messages (optional) |
| `channels:read` | List channels |

## 3. Enable Event Subscriptions

Go to **Event Subscriptions** in the left sidebar:

1. Toggle **Enable Events** to **On**
2. **Request URL**: enter `https://<your-domain>/slack/events`
   - For local development, use ngrok to create a tunnel (see Step 6 below)
   - The URL verification will succeed once Scribe is running
3. Under **Subscribe to bot events**, add:
   - `message.channels` — messages in public channels
   - `message.groups` — messages in private channels
   - `message.im` — direct messages (optional)
4. Click **Save Changes**

## 4. Install the App to Your Workspace

1. Go to **OAuth & Permissions** → click **Install to Workspace**
2. Review and approve the permissions
3. Copy the **Signing Secret**: go to **Basic Information** → **App Credentials** → **Signing Secret**

## 5. Set Environment Variables

```bash
# Required
export SLACK_SIGNING_SECRET="<Signing Secret from Step 4>"
export ENVECTOR_ENDPOINT="<enVector Cloud endpoint>"
export ENVECTOR_API_KEY="<enVector API key>"

# Optional: enables Tier 2/3 LLM pipeline
export ANTHROPIC_API_KEY="<Anthropic API key>"

# Optional: server port (default: 8080)
export SCRIBE_PORT=8080
```

Alternatively, create a config file at `~/.rune/config.json`:

```json
{
  "state": "active",
  "scribe": {
    "slack_webhook_port": 8080,
    "slack_signing_secret": "<Signing Secret>",
    "similarity_threshold": 0.35,
    "auto_capture_threshold": 0.7,
    "tier2_enabled": true
  },
  "envector": {
    "endpoint": "<enVector Cloud endpoint>",
    "api_key": "<enVector API key>"
  },
  "retriever": {
    "anthropic_api_key": "<Anthropic API key>"
  }
}
```

## 6. Expose Local Server with ngrok

```bash
# Install ngrok (macOS)
brew install ngrok

# Authenticate (one-time setup, get your token from https://dashboard.ngrok.com)
ngrok config add-authtoken <your-token>

# Start tunnel
ngrok http 8080
```

Copy the `https://xxxx.ngrok-free.app` URL from ngrok output and paste it into the Slack Event Subscription Request URL field:

```
https://xxxx.ngrok-free.app/slack/events
```

## 7. Start the Scribe Server

```bash
cd rune/
PYTHONPATH=. python -m agents.scribe.server
```

Or use the helper script:

```bash
bash rune/scripts/setup-slack-app.sh
```

## 8. Verify It Works

Send a message in any channel the bot has access to. Try something like:

> We decided to use PostgreSQL instead of MongoDB because we need ACID compliance for financial transactions.

Check the Scribe server logs for capture confirmation:

```
INFO:rune.scribe:Tier 1 PASS (score: 0.72, pattern: "we decided to use...")
INFO:rune.scribe:Tier 3 built record: dec_2026-02-13_arch_... (certainty: partially_supported)
INFO:rune.scribe:Stored: dec_2026-02-13_arch_...
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| URL verification fails | Make sure Scribe is running and the ngrok URL is correct |
| Messages not captured | Lower `SCRIBE_THRESHOLD` (default 0.5, try 0.3) |
| Too many Tier 2 REJECTs | Tier 2 is auto-skipped when `ANTHROPIC_API_KEY` is not set |
| enVector insert fails | Check `ENVECTOR_ENDPOINT` and `ENVECTOR_API_KEY` |
| Signing Secret error | Set `SLACK_SIGNING_SECRET=""` to skip verification during development |
