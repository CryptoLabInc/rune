---
description: Delete a captured decision record
allowed-tools: mcp__envector__delete_capture, mcp__envector__capture_history
---

# /rune:delete — Delete Captured Record

Soft-delete a captured decision record by marking it as reverted.

## Steps

1. If $ARGUMENTS is empty:
   - Call `capture_history` with limit=10 to show recent captures
   - Ask user which record to delete by ID

2. If $ARGUMENTS contains a record_id:
   - Confirm with user: "Delete record `<record_id>`? This marks it as reverted (soft-delete). It will be heavily demoted in search results."

3. If user confirms:
   - Call `delete_capture` MCP tool with the record_id
   - On success: "Deleted: [title] (ID: [record_id]). Record marked as reverted."
   - On error: Show the error message

4. If user declines:
   - "Cancelled. Record unchanged."
