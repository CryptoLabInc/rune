"""
Slack Handler

Handles Slack webhook events and converts them to Messages.
"""

import hmac
import hashlib
import re
import time
import logging
from typing import Optional, Dict, Any, List, Callable, Awaitable

from .base import BaseHandler, Message

logger = logging.getLogger("rune.scribe")


class SlackHandler(BaseHandler):
    """
    Handler for Slack Events API webhooks.

    Processes:
    - app_mention events (@Rune mentions)

    Ignores:
    - Bot messages
    - Message deletions
    - Reaction events
    """

    def __init__(self, signing_secret: str = ""):
        super().__init__("slack")
        self._signing_secret = signing_secret

    # -------------------------------------------------------------------------
    # Public API
    # -------------------------------------------------------------------------

    async def parse_event(self, raw_data: Dict[str, Any]) -> Optional[Message]:
        """Parse a raw Slack webhook payload into a Message, or None to ignore."""
        logger.info("[slack] Incoming event — top-level type: %s", raw_data.get("type"))

        if raw_data.get("type") == "url_verification":
            logger.info("[slack] URL verification challenge, skipping")
            return None

        if raw_data.get("type") != "event_callback":
            logger.info("[slack] Not an event_callback, ignoring (type=%s)", raw_data.get("type"))
            return None

        event = raw_data.get("event", {})
        event_type = event.get("type", "")
        logger.info(
            "[slack] event_callback received — event.type=%s user=%s channel=%s",
            event_type, event.get("user"), event.get("channel"),
        )

        return self._route_event(event_type, event, raw_data)

    def verify_signature(self, body: bytes, signature: str, timestamp: str) -> bool:
        """Verify Slack HMAC-SHA256 request signature."""
        if not self._signing_secret:
            return True

        if not signature or not timestamp:
            return False

        try:
            if abs(time.time() - int(timestamp)) > 300:
                return False
        except ValueError:
            return False

        basestring = f"v0:{timestamp}:{body.decode('utf-8')}"
        expected = "v0=" + hmac.new(
            self._signing_secret.encode("utf-8"),
            basestring.encode("utf-8"),
            hashlib.sha256,
        ).hexdigest()

        return hmac.compare_digest(expected, signature)

    def should_process(self, message: Message) -> bool:
        """Slack-specific filter: drop messages that are only mentions or URLs."""
        if not super().should_process(message):
            return False

        text = message.text.strip()
        text = re.sub(r"<@U[A-Z0-9]+>", "", text).strip()
        if len(text) < 15:
            return False

        text = re.sub(r"<https?://[^>]+>", "", text).strip()
        if len(text) < 15:
            return False

        return True

    # -------------------------------------------------------------------------
    # Event routing
    # -------------------------------------------------------------------------

    def _route_event(
        self,
        event_type: str,
        event: Dict[str, Any],
        raw_data: Dict[str, Any],
    ) -> Optional[Message]:
        """Dispatch to the appropriate handler based on event_type."""
        handlers = {
            "app_mention": self._handle_app_mention,
        }

        handler = handlers.get(event_type)
        if handler:
            return handler(event, raw_data)

        logger.info("[slack] Unhandled event type: %s", event_type)
        return None

    # -------------------------------------------------------------------------
    # Event handlers
    # -------------------------------------------------------------------------

    def _handle_app_mention(
        self,
        event: Dict[str, Any],
        raw_data: Dict[str, Any],
    ) -> Optional[Message]:
        """Handle @Rune mention events."""
        message = self._parse_message_event(event, raw_data)
        if message:
            logger.info("[slack] app_mention parsed — user=%s text=%r", message.user, message.text)
        else:
            logger.info("[slack] app_mention received but _parse_message_event returned None")
        return message

    # -------------------------------------------------------------------------
    # Parsing helpers
    # -------------------------------------------------------------------------

    def _parse_message_event(
        self,
        event: Dict[str, Any],
        raw_data: Dict[str, Any],
    ) -> Optional[Message]:
        """Parse a standard message or app_mention event into a Message."""
        if event.get("bot_id") or event.get("subtype") == "bot_message":
            return None

        ignored_subtypes = {
            "channel_join", "channel_leave", "channel_topic",
            "channel_purpose", "channel_name", "message_deleted",
            "file_share", "thread_broadcast",
        }
        if event.get("subtype") in ignored_subtypes:
            return None

        text = event.get("text", "")
        user = event.get("user", "")
        channel = event.get("channel", "")
        ts = event.get("ts", "")
        thread_ts = event.get("thread_ts")

        url = None
        team_id = raw_data.get("team_id")
        if team_id and channel and ts:
            url = f"https://slack.com/archives/{channel}/p{ts.replace('.', '')}"

        return Message(
            text=text,
            user=user,
            channel=channel,
            source="slack",
            timestamp=ts,
            thread_ts=thread_ts,
            url=url,
            is_bot=False,
            mentions=self._extract_mentions(text),
            reactions=event.get("reactions", []),
            raw_data=event,
        )

    def _extract_mentions(self, text: str) -> List[str]:
        """Extract Slack user IDs from mention tags (<@U12345678>)."""
        return re.findall(r"<@(U[A-Z0-9]+)>", text)
