"""
Source Handlers

Extensible handlers for different event sources.
Each handler converts source-specific events to a common Message format.

Available Handlers:
- SlackHandler: Slack webhook events

Future Handlers:
- GitHubHandler: PR/Issue webhooks
- NotionHandler: Database change polling
"""

from .base import BaseHandler, Message
from .slack import SlackHandler

__all__ = [
    "BaseHandler",
    "Message",
    "SlackHandler",
]
