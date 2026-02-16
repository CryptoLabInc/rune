"""
Base Handler

Abstract base class for source-specific event handlers.
Provides a common interface for converting events to Messages.
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Optional, Dict, Any
from datetime import datetime


@dataclass
class Message:
    """
    Common message format for all sources.

    This is the standardized format that Scribe works with,
    regardless of the original source (Slack, GitHub, etc.).
    """
    text: str
    user: str
    channel: str
    source: str  # "slack", "github", "notion", etc.
    timestamp: str
    thread_ts: Optional[str] = None
    url: Optional[str] = None
    is_bot: bool = False
    mentions: list = field(default_factory=list)
    reactions: list = field(default_factory=list)
    attachments: list = field(default_factory=list)
    raw_data: Optional[Dict[str, Any]] = None

    @property
    def datetime(self) -> Optional[datetime]:
        """Parse timestamp to datetime"""
        try:
            return datetime.fromtimestamp(float(self.timestamp))
        except (ValueError, TypeError):
            return None

    @property
    def is_valid(self) -> bool:
        """Check if message has minimum required fields"""
        return bool(self.text and self.text.strip())


class BaseHandler(ABC):
    """
    Abstract base class for source handlers.

    Each handler must implement:
    - parse_event: Convert raw event to Message
    - verify_signature: Verify webhook signature (if applicable)
    """

    def __init__(self, source_name: str):
        """
        Initialize handler.

        Args:
            source_name: Name of the source (e.g., "slack", "github")
        """
        self.source_name = source_name

    @abstractmethod
    async def parse_event(self, raw_data: Dict[str, Any]) -> Optional[Message]:
        """
        Parse raw event data into a Message.

        Args:
            raw_data: Raw event data from the source

        Returns:
            Message object or None if event should be ignored
        """
        pass

    @abstractmethod
    def verify_signature(
        self,
        body: bytes,
        signature: str,
        timestamp: str
    ) -> bool:
        """
        Verify the webhook signature.

        Args:
            body: Raw request body
            signature: Signature from headers
            timestamp: Timestamp from headers

        Returns:
            True if signature is valid
        """
        pass

    def should_process(self, message: Message) -> bool:
        """
        Check if message should be processed.

        Default implementation filters out:
        - Bot messages
        - Empty messages
        - Very short messages

        Override in subclass for source-specific filtering.

        Args:
            message: Parsed message

        Returns:
            True if message should be processed
        """
        # Skip invalid messages
        if not message.is_valid:
            return False

        # Skip bot messages
        if message.is_bot:
            return False

        # Skip very short messages
        if len(message.text.strip()) < 20:
            return False

        return True

    def extract_thread_context(self, raw_data: Dict[str, Any]) -> Optional[str]:
        """
        Extract thread context if message is part of a thread.

        Override in subclass for source-specific implementation.

        Args:
            raw_data: Raw event data

        Returns:
            Thread context or None
        """
        return None
