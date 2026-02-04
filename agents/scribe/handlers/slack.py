"""
Slack Handler

Handles Slack webhook events and converts them to Messages.
"""

import hmac
import hashlib
import time
from typing import Optional, Dict, Any, List

from .base import BaseHandler, Message


class SlackHandler(BaseHandler):
    """
    Handler for Slack Events API webhooks.

    Processes:
    - message events (new messages in channels)
    - message_changed events (edited messages)

    Ignores:
    - Bot messages
    - Message deletions
    - Reaction events (for now)
    """

    def __init__(self, signing_secret: str = ""):
        """
        Initialize Slack handler.

        Args:
            signing_secret: Slack signing secret for verification
        """
        super().__init__("slack")
        self._signing_secret = signing_secret

    async def parse_event(self, raw_data: Dict[str, Any]) -> Optional[Message]:
        """
        Parse Slack event into Message.

        Args:
            raw_data: Raw Slack event data

        Returns:
            Message object or None if event should be ignored
        """
        # Handle URL verification challenge
        if raw_data.get("type") == "url_verification":
            return None

        # Handle event callback
        if raw_data.get("type") != "event_callback":
            return None

        event = raw_data.get("event", {})
        event_type = event.get("type", "")

        # Handle message events
        if event_type == "message":
            return self._parse_message_event(event, raw_data)

        # Handle message changed events
        if event_type == "message" and event.get("subtype") == "message_changed":
            return self._parse_message_changed_event(event, raw_data)

        return None

    def _parse_message_event(
        self,
        event: Dict[str, Any],
        raw_data: Dict[str, Any]
    ) -> Optional[Message]:
        """Parse a standard message event"""
        # Skip bot messages
        if event.get("bot_id") or event.get("subtype") == "bot_message":
            return None

        # Skip message subtypes we don't care about
        ignored_subtypes = [
            "channel_join", "channel_leave", "channel_topic",
            "channel_purpose", "channel_name", "message_deleted",
            "file_share", "thread_broadcast",
        ]
        if event.get("subtype") in ignored_subtypes:
            return None

        text = event.get("text", "")
        user = event.get("user", "")
        channel = event.get("channel", "")
        ts = event.get("ts", "")
        thread_ts = event.get("thread_ts")

        # Extract mentions
        mentions = self._extract_mentions(text)

        # Extract reactions (if available)
        reactions = event.get("reactions", [])

        # Build URL if team info available
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
            mentions=mentions,
            reactions=reactions,
            raw_data=event,
        )

    def _parse_message_changed_event(
        self,
        event: Dict[str, Any],
        raw_data: Dict[str, Any]
    ) -> Optional[Message]:
        """Parse a message_changed event"""
        message = event.get("message", {})

        # Skip bot messages
        if message.get("bot_id"):
            return None

        return Message(
            text=message.get("text", ""),
            user=message.get("user", ""),
            channel=event.get("channel", ""),
            source="slack",
            timestamp=message.get("ts", ""),
            thread_ts=message.get("thread_ts"),
            is_bot=False,
            mentions=self._extract_mentions(message.get("text", "")),
            raw_data=event,
        )

    def _extract_mentions(self, text: str) -> List[str]:
        """Extract user mentions from text"""
        import re
        # Slack mentions format: <@U12345678>
        matches = re.findall(r'<@(U[A-Z0-9]+)>', text)
        return matches

    def verify_signature(
        self,
        body: bytes,
        signature: str,
        timestamp: str
    ) -> bool:
        """
        Verify Slack request signature.

        Args:
            body: Raw request body
            signature: X-Slack-Signature header
            timestamp: X-Slack-Request-Timestamp header

        Returns:
            True if signature is valid
        """
        if not self._signing_secret:
            # Skip verification if no secret configured
            return True

        if not signature or not timestamp:
            return False

        # Check timestamp is recent (within 5 minutes)
        try:
            ts = int(timestamp)
            if abs(time.time() - ts) > 300:
                return False
        except ValueError:
            return False

        # Compute expected signature
        sig_basestring = f"v0:{timestamp}:{body.decode('utf-8')}"
        expected_sig = "v0=" + hmac.new(
            self._signing_secret.encode('utf-8'),
            sig_basestring.encode('utf-8'),
            hashlib.sha256
        ).hexdigest()

        # Compare signatures
        return hmac.compare_digest(expected_sig, signature)

    def should_process(self, message: Message) -> bool:
        """
        Check if Slack message should be processed.

        Additional Slack-specific filtering.
        """
        if not super().should_process(message):
            return False

        # Skip messages that are just mentions or links
        text = message.text.strip()

        # Skip if mostly mentions
        import re
        clean_text = re.sub(r'<@U[A-Z0-9]+>', '', text).strip()
        if len(clean_text) < 15:
            return False

        # Skip if mostly URLs
        clean_text = re.sub(r'<https?://[^>]+>', '', clean_text).strip()
        if len(clean_text) < 15:
            return False

        return True

    def format_channel_name(self, channel_id: str, channel_name: str = None) -> str:
        """Format channel for display"""
        if channel_name:
            return f"#{channel_name}"
        return f"#{channel_id}"

    def is_thread_reply(self, message: Message) -> bool:
        """Check if message is a thread reply"""
        return message.thread_ts is not None and message.thread_ts != message.timestamp

    def is_url_verification(self, raw_data: Dict[str, Any]) -> bool:
        """Check if request is URL verification"""
        return raw_data.get("type") == "url_verification"

    def get_challenge(self, raw_data: Dict[str, Any]) -> Optional[str]:
        """Get challenge for URL verification"""
        if self.is_url_verification(raw_data):
            return raw_data.get("challenge")
        return None
