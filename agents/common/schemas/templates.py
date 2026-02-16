"""
Payload Text Templates

Renders DecisionRecord to Markdown format for embedding.
The payload.text is the SINGLE SOURCE OF TRUTH for memory reproduction.
"""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .decision_record import DecisionRecord


PAYLOAD_TEMPLATE = """# Decision Record: {title}
ID: {id}
Status: {status} | Sensitivity: {sensitivity} | Domain: {domain}
When/Where: {when} | {where}

## Decision
{what}

## Problem
{problem}

## Alternatives Considered
{alternatives}

## Why (Rationale)
{rationale_summary}
Certainty: {certainty}

## Trade-offs
{trade_offs}

## Assumptions
{assumptions}

## Risks & Mitigations
{risks}

## Evidence (Quotes)
{evidence_block}

## Links
{links}

## Tags
{tags}
"""


def _format_alternatives(alternatives: list, chosen: str) -> str:
    """Format alternatives list with chosen marker"""
    if not alternatives:
        return "- (none documented)"

    lines = []
    for alt in alternatives:
        if alt.lower() == chosen.lower() or chosen.lower() in alt.lower():
            lines.append(f"- {alt} (chosen)")
        else:
            lines.append(f"- {alt}")
    return "\n".join(lines)


def _format_trade_offs(trade_offs: list) -> str:
    """Format trade-offs list"""
    if not trade_offs:
        return "- (none documented)"
    return "\n".join(f"- {t}" for t in trade_offs)


def _format_assumptions(assumptions: list) -> str:
    """Format assumptions with confidence"""
    if not assumptions:
        return "- (none documented)"

    lines = []
    for a in assumptions:
        conf = getattr(a, 'confidence', 0.5)
        lines.append(f"- {a.assumption} (confidence: {conf:.1f})")
    return "\n".join(lines)


def _format_risks(risks: list) -> str:
    """Format risks with mitigations"""
    if not risks:
        return "- (none documented)"

    lines = []
    for r in risks:
        mitigation = getattr(r, 'mitigation', None) or "TBD"
        lines.append(f"- Risk: {r.risk}\n  Mitigation: {mitigation}")
    return "\n".join(lines)


def _format_evidence(evidence: list) -> str:
    """Format evidence with quotes and sources"""
    if not evidence:
        return "(no evidence recorded)"

    lines = []
    for i, e in enumerate(evidence, 1):
        source_type = e.source.type.value if hasattr(e.source.type, 'value') else str(e.source.type)
        source_url = e.source.url or "(no url)"
        source_pointer = e.source.pointer or ""

        lines.append(f"{i}) Claim: {e.claim}")
        lines.append(f'   Quote: "{e.quote}"')
        lines.append(f"   Source: {source_type} {source_url}")
        if source_pointer:
            lines.append(f"   Pointer: {source_pointer}")
        lines.append("")

    return "\n".join(lines).strip()


def _format_links(links: list) -> str:
    """Format related links"""
    if not links:
        return "- (none)"

    lines = []
    for link in links:
        rel = link.get('rel', 'link')
        url = link.get('url', '')
        lines.append(f"- {rel}: {url}")
    return "\n".join(lines)


def _format_tags(tags: list) -> str:
    """Format tags as comma-separated"""
    if not tags:
        return "(none)"
    return ", ".join(tags)


def render_payload_text(record: "DecisionRecord") -> str:
    """
    Render a DecisionRecord to payload.text (Markdown).

    This text is used for:
    1. Embedding generation (for enVector search)
    2. Memory reproduction (human-readable context)

    The text should be self-contained - reading just this text
    should give full understanding of the decision.
    """
    # Extract values with safe defaults
    domain = record.domain.value if hasattr(record.domain, 'value') else str(record.domain)
    sensitivity = record.sensitivity.value if hasattr(record.sensitivity, 'value') else str(record.sensitivity)
    status = record.status.value if hasattr(record.status, 'value') else str(record.status)
    certainty = record.why.certainty.value if hasattr(record.why.certainty, 'value') else str(record.why.certainty)

    # Format complex fields
    alternatives = _format_alternatives(
        record.context.alternatives,
        record.context.chosen
    )
    trade_offs = _format_trade_offs(record.context.trade_offs)
    assumptions = _format_assumptions(record.context.assumptions)
    risks = _format_risks(record.context.risks)
    evidence_block = _format_evidence(record.evidence)
    links = _format_links(record.links)
    tags = _format_tags(record.tags)

    # Build rationale with missing info if applicable
    rationale = record.why.rationale_summary or "(no rationale documented)"
    if record.why.missing_info:
        rationale += "\n\nMissing Information:\n" + "\n".join(f"- {m}" for m in record.why.missing_info)

    # Render template
    text = PAYLOAD_TEMPLATE.format(
        title=record.title,
        id=record.id,
        status=status,
        sensitivity=sensitivity,
        domain=domain,
        when=record.decision.when or "(unknown)",
        where=record.decision.where or "(unknown)",
        what=record.decision.what,
        problem=record.context.problem or "(not documented)",
        alternatives=alternatives,
        rationale_summary=rationale,
        certainty=certainty,
        trade_offs=trade_offs,
        assumptions=assumptions,
        risks=risks,
        evidence_block=evidence_block,
        links=links,
        tags=tags,
    )

    # Clean up multiple blank lines
    while "\n\n\n" in text:
        text = text.replace("\n\n\n", "\n\n")

    return text.strip()


def render_compact_payload(record: "DecisionRecord") -> str:
    """
    Render a compact version for search result previews.
    """
    domain = record.domain.value if hasattr(record.domain, 'value') else str(record.domain)
    certainty = record.why.certainty.value if hasattr(record.why.certainty, 'value') else str(record.why.certainty)

    return f"""**{record.title}** ({record.id})
Domain: {domain} | Certainty: {certainty}

{record.decision.what}

Why: {record.why.rationale_summary or '(no rationale)'}
"""


# Localized headers for display rendering (not for payload.text / embedding)
PAYLOAD_HEADERS = {
    "en": {
        "decision_record": "Decision Record",
        "decision": "Decision",
        "problem": "Problem",
        "alternatives": "Alternatives Considered",
        "rationale": "Why (Rationale)",
        "certainty": "Certainty",
        "trade_offs": "Trade-offs",
        "assumptions": "Assumptions",
        "risks": "Risks & Mitigations",
        "evidence": "Evidence (Quotes)",
        "links": "Links",
        "tags": "Tags",
    },
    "ko": {
        "decision_record": "결정 기록",
        "decision": "결정 사항",
        "problem": "문제",
        "alternatives": "검토한 대안",
        "rationale": "근거 (이유)",
        "certainty": "확실성",
        "trade_offs": "트레이드오프",
        "assumptions": "가정",
        "risks": "리스크 및 대응",
        "evidence": "증거 (인용)",
        "links": "링크",
        "tags": "태그",
    },
    "ja": {
        "decision_record": "決定記録",
        "decision": "決定事項",
        "problem": "問題",
        "alternatives": "検討した代替案",
        "rationale": "根拠（理由）",
        "certainty": "確実性",
        "trade_offs": "トレードオフ",
        "assumptions": "仮定",
        "risks": "リスクと対策",
        "evidence": "証拠（引用）",
        "links": "リンク",
        "tags": "タグ",
    },
}


def render_display_text(record: "DecisionRecord", language: str = "en") -> str:
    """Render a DecisionRecord with localized headers for user display.

    Unlike render_payload_text() (which is always English for embedding consistency),
    this function uses localized section headers for human-readable presentation.

    Args:
        record: DecisionRecord to render
        language: ISO 639-1 language code

    Returns:
        Localized markdown text for display
    """
    headers = PAYLOAD_HEADERS.get(language, PAYLOAD_HEADERS["en"])

    domain = record.domain.value if hasattr(record.domain, 'value') else str(record.domain)
    sensitivity = record.sensitivity.value if hasattr(record.sensitivity, 'value') else str(record.sensitivity)
    status = record.status.value if hasattr(record.status, 'value') else str(record.status)
    certainty = record.why.certainty.value if hasattr(record.why.certainty, 'value') else str(record.why.certainty)

    alternatives = _format_alternatives(
        record.context.alternatives,
        record.context.chosen
    )
    trade_offs = _format_trade_offs(record.context.trade_offs)
    assumptions = _format_assumptions(record.context.assumptions)
    risks = _format_risks(record.context.risks)
    evidence_block = _format_evidence(record.evidence)
    links = _format_links(record.links)
    tags = _format_tags(record.tags)

    rationale = record.why.rationale_summary or "(no rationale documented)"
    if record.why.missing_info:
        rationale += "\n\nMissing Information:\n" + "\n".join(f"- {m}" for m in record.why.missing_info)

    text = f"""# {headers['decision_record']}: {record.title}
ID: {record.id}
Status: {status} | Sensitivity: {sensitivity} | Domain: {domain}
When/Where: {record.decision.when or '(unknown)'} | {record.decision.where or '(unknown)'}

## {headers['decision']}
{record.decision.what}

## {headers['problem']}
{record.context.problem or '(not documented)'}

## {headers['alternatives']}
{alternatives}

## {headers['rationale']}
{rationale}
{headers['certainty']}: {certainty}

## {headers['trade_offs']}
{trade_offs}

## {headers['assumptions']}
{assumptions}

## {headers['risks']}
{risks}

## {headers['evidence']}
{evidence_block}

## {headers['links']}
{links}

## {headers['tags']}
{tags}
"""

    while "\n\n\n" in text:
        text = text.replace("\n\n\n", "\n\n")

    return text.strip()
