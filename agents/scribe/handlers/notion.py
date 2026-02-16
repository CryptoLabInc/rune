"""
Notion Handler

Handles Notion webhook events and converts them to Messages.
Supports page.created, page.updated, and database.updated events
via Notion's Send Webhooks API.
"""

import hmac
import hashlib
import time
import logging
from typing import Optional, Dict, Any, List

from .base import BaseHandler, Message

logger = logging.getLogger("rune.scribe.notion")


class NotionHandler(BaseHandler):
    """
    Handler for Notion webhook events.

    Processes:
    - page.created: New page creation
    - page.updated: Page content or property updates
    - database.updated: Database schema or entry changes

    Ignores:
    - Bot/automation edits
    - Template instantiation
    - Very short pages (likely stubs)
    """

    def __init__(self, signing_secret: str = ""):
        """
        Initialize Notion handler.

        Args:
            signing_secret: Notion webhook secret for HMAC-SHA256 verification
        """
        super().__init__("notion")
        self._signing_secret = signing_secret

    async def parse_event(self, raw_data: Dict[str, Any]) -> Optional[Message]:
        """
        Parse Notion webhook event into Message.

        Args:
            raw_data: Raw Notion webhook payload

        Returns:
            Message object or None if event should be ignored
        """
        event_type = raw_data.get("type", "")

        if event_type in ("page.created", "page.updated"):
            return self._parse_page_event(raw_data)
        elif event_type == "database.updated":
            return self._parse_database_event(raw_data)

        return None

    def _parse_page_event(self, raw_data: Dict[str, Any]) -> Optional[Message]:
        """Parse a page.created or page.updated event"""
        page = raw_data.get("data", raw_data.get("page", {}))

        # Extract title from properties
        title = self._extract_title(page)

        # Extract rich_text content from page blocks (if included)
        body = self._extract_body(raw_data)

        # Combine title + body
        text = title
        if body:
            text = f"{title}\n\n{body}" if title else body

        if not text:
            return None

        # Extract user
        user = self._extract_user(page)

        # Extract parent context as channel
        channel = self._extract_parent_name(page)

        # Extract timestamp — prefer last_edited_time, fall back to created_time
        timestamp = self._extract_timestamp(page)

        # Page URL
        url = page.get("url")

        # Detect bot/automation edits
        is_bot = self._is_bot_edit(page)

        return Message(
            text=text,
            user=user,
            channel=channel,
            source="notion",
            timestamp=timestamp,
            thread_ts=None,
            url=url,
            is_bot=is_bot,
            raw_data=raw_data,
        )

    def _parse_database_event(self, raw_data: Dict[str, Any]) -> Optional[Message]:
        """Parse a database.updated event"""
        database = raw_data.get("data", raw_data.get("database", {}))

        # Database title
        title_parts = database.get("title", [])
        title = "".join(
            t.get("plain_text", "") for t in title_parts
        )

        if not title:
            return None

        text = f"Database updated: {title}"

        # Description if available
        description_parts = database.get("description", [])
        if description_parts:
            desc = "".join(t.get("plain_text", "") for t in description_parts)
            if desc:
                text = f"{text}\n\n{desc}"

        user = self._extract_user(database)
        timestamp = self._extract_timestamp(database)

        return Message(
            text=text,
            user=user,
            channel=title,
            source="notion",
            timestamp=timestamp,
            thread_ts=None,
            url=database.get("url"),
            is_bot=self._is_bot_edit(database),
            raw_data=raw_data,
        )

    def _extract_title(self, page: Dict[str, Any]) -> str:
        """Extract page title from properties"""
        properties = page.get("properties", {})
        for prop in properties.values():
            if prop.get("type") == "title":
                title_parts = prop.get("title", [])
                return "".join(t.get("plain_text", "") for t in title_parts)
        return ""

    def _extract_body(self, raw_data: Dict[str, Any]) -> str:
        """Extract body text from rich_text blocks if included in payload"""
        blocks = raw_data.get("blocks", raw_data.get("children", []))
        parts: List[str] = []

        for block in blocks:
            block_type = block.get("type", "")
            block_data = block.get(block_type, {})

            # Extract rich_text from common block types
            rich_text = block_data.get("rich_text", [])
            text = "".join(t.get("plain_text", "") for t in rich_text)
            if text:
                parts.append(text)

        return "\n".join(parts)

    def _extract_user(self, obj: Dict[str, Any]) -> str:
        """Extract user ID from last_edited_by or created_by"""
        editor = obj.get("last_edited_by", obj.get("created_by", {}))
        return editor.get("id", "unknown")

    def _extract_parent_name(self, page: Dict[str, Any]) -> str:
        """Extract parent context (database name or parent page) as channel"""
        parent = page.get("parent", {})
        parent_type = parent.get("type", "")

        if parent_type == "database_id":
            return f"db:{parent.get('database_id', 'unknown')}"
        elif parent_type == "page_id":
            return f"page:{parent.get('page_id', 'unknown')}"
        elif parent_type == "workspace":
            return "workspace"

        return "notion"

    def _extract_timestamp(self, obj: Dict[str, Any]) -> str:
        """Extract timestamp, converting ISO to unix epoch string"""
        iso_time = obj.get("last_edited_time", obj.get("created_time", ""))
        if iso_time:
            try:
                from datetime import datetime, timezone
                # Notion uses ISO 8601 format
                dt = datetime.fromisoformat(iso_time.replace("Z", "+00:00"))
                return str(dt.timestamp())
            except (ValueError, TypeError):
                pass
        return str(time.time())

    def _is_bot_edit(self, obj: Dict[str, Any]) -> bool:
        """Check if edit was made by a bot/integration"""
        editor = obj.get("last_edited_by", obj.get("created_by", {}))
        return editor.get("type") == "bot"

    def verify_signature(
        self,
        body: bytes,
        signature: str,
        timestamp: str
    ) -> bool:
        """
        Verify Notion webhook signature using HMAC-SHA256.

        Args:
            body: Raw request body
            signature: X-Notion-Signature header value
            timestamp: X-Notion-Timestamp header value (unused for Notion,
                       kept for BaseHandler interface compatibility)

        Returns:
            True if signature is valid
        """
        if not self._signing_secret:
            return True

        if not signature:
            return False

        expected_sig = hmac.new(
            self._signing_secret.encode("utf-8"),
            body,
            hashlib.sha256
        ).hexdigest()

        return hmac.compare_digest(expected_sig, signature)

    def should_process(self, message: Message) -> bool:
        """
        Check if Notion message should be processed.

        Filters out:
        - Bot/automation edits (via base class)
        - Very short content (via base class)
        - Template instantiation (title starts with common template markers)
        """
        if not super().should_process(message):
            return False

        title_line = message.text.split("\n")[0].strip()

        # Skip untitled/template pages
        skip_prefixes = ("Untitled", "Template:", "[Template]", "Copy of ")
        if title_line.startswith(skip_prefixes):
            return False

        return True
