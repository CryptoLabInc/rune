"""
Source Handlers

Extensible handlers for different event sources.
Each handler converts source-specific events to a common Message format.

Available Handlers:
- SlackHandler: Slack webhook events
- NotionHandler: Notion webhook events

Future Handlers:
- GitHubHandler: PR/Issue webhooks
"""

from .base import BaseHandler, Message
from .slack import SlackHandler
from .notion import NotionHandler

__all__ = [
    "BaseHandler",
    "Message",
    "SlackHandler",
    "NotionHandler",
]
