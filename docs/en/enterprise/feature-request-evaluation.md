English | **[Русский](../../ru/enterprise/feature-request-evaluation.md)**

# Feature request evaluation template

**Audience:** Product maintainers, partners, enterprise account owners  
**Policy:** [Community and Enterprise lifecycle](./community-enterprise-lifecycle.md)

Copy this template into an issue, private tracker entry, or shared document. Complete all sections before a ship/no-ship decision.

---

## 1. Request metadata

| Field | Value |
|-------|-------|
| **Request ID** | `FR-YYYY-NNN` |
| **Title** | |
| **Date submitted** | |
| **Submitted by** | Name · role (customer / partner / contributor / internal) |
| **Customer / account** | Company name (or `N/A — community`) |
| **Segment** | `SMB` · `Mid-market` · `Enterprise` · `Public sector` · `Partner` · `Community` |
| **Urgency** | `Low` · `Medium` · `High` · `Critical (deal at risk)` |
| **Target release** | Quarter or date requested by submitter |
| **Links** | Issue URL, RFP excerpt, design doc |

### Summary (2–4 sentences)

_Describe the problem, who is affected, and the proposed outcome._

---

## 2. Scope and fit

| Question | Answer |
|----------|--------|
| Is this in DataSafeS3 product scope (governed self-hosted storage)? | Yes / No / Unclear |
| Overlaps existing roadmap item? | Link or `None` |
| Estimated engineering size | `S` · `M` · `L` · `XL` |
| CE documentation impact? | EN + RU updates required? Yes / No |

---

## 3. Scoring checklist

Score each criterion **Yes (1)** or **No (0)**. Enterprise-first requires **≥ 2 of 4** Yes.

| # | Criterion | Y (1) / N (0) | Evidence / notes |
|---|-----------|---------------|------------------|
| 1 | **Paying customers** — funded or required by paying Enterprise customer(s) within two quarters | | |
| 2 | **Deal blocker** — blocks active enterprise procurement or renewal | | |
| 3 | **High support cost** — CE-wide release would create disproportionate support burden before patterns are proven | | |
| 4 | **Narrow CE audience** — unlikely 80% of CE deployments need this within six months | | |
| | **Total (max 4)** | **/4** | |

### Additional factors (optional)

| Factor | Notes |
|--------|-------|
| Security / compliance driver | |
| Breaking API or schema change | |
| Dependency on third-party service | |
| Permanent Enterprise-only? (support/SLA/cert only) | Yes / No |

---

## 4. Decision

Select **one** primary outcome:

| Option | When to choose |
|--------|----------------|
| **Community now** | Total score 0–1, broad CE value, acceptable risk |
| **Enterprise first** | Total score ≥ 2, commercial signal justifies staged delivery |
| **Backlog** | Valid but insufficient priority or capacity |
| **Decline** | Out of scope, conflicts with CE principles, or unsustainable |

**Primary decision:** `Community now` · `Enterprise first` · `Backlog` · `Decline`

**Rationale (required):**

---

## 5. Timeline (if Enterprise first)

Complete when decision is **Enterprise first**. Leave blank otherwise.

| Milestone | Target quarter | Owner |
|-----------|----------------|-------|
| Enterprise preview | | |
| Enterprise GA | | |
| **Target CE migration quarter** | | |
| Community GA (Apache-2.0 in main) | | |

**Maturity calendar entry drafted?** Yes / No  
**Public announcement plan** | Preview blog / release notes only / customer-only |

### CE migration prerequisites

- [ ] Production use under Enterprise for 3–6 months
- [ ] Support runbook and operator docs ready
- [ ] EN + RU user-facing docs drafted
- [ ] No unresolved security or compliance blockers for CE

---

## 6. Sign-off

| Role | Name | Date | Decision agreed (Y/N) |
|------|------|------|------------------------|
| **Product / maintainer** | | | |
| **Engineering lead** | | | |
| **Enterprise account** (if applicable) | | | |
| **Documentation** (if CE or CE migration planned) | | | |

**Final status:** `Approved` · `Approved with conditions` · `Rejected` · `Deferred`

**Conditions / follow-ups:**

---

## Quick reference

| Score | Typical decision |
|-------|------------------|
| 0–1 | Community now (if in scope) |
| 2–4 | Enterprise first (publish CE migration quarter) |
| Any + out of scope | Decline |
| Valid + no capacity | Backlog |

Permanent Enterprise-only items (support, SLA, certified builds, professional services) **do not** use this template for CE migration — mark **Enterprise only** in section 5 and skip CE quarter.

See [Community and Enterprise lifecycle](./community-enterprise-lifecycle.md) for policy details.
