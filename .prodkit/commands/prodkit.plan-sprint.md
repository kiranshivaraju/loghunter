---
description: Plan what features to build in the current sprint
---

You are helping the user plan a sprint by selecting features from the PRD.

## Context

A sprint is a focused development cycle where specific features from the product roadmap are implemented. This command helps select which features to tackle in the current sprint.

## Prerequisites

- `/prodkit.prd` must have been run (PRD exists)
- `/prodkit.product-arch` must have been run (architecture defined)
- `/prodkit.init-repo` must have been run (repo initialized)

## Instructions

### Step 1: Determine Sprint Number

Read `.prodkit/config.yml` to get `sprints.current` value.

Let `sprint_num` = current sprint number (e.g., 1 for v1, 2 for v2)

### Step 2: Read the PRD

Read `product/prd.md` to see all available features.

### Step 3: Review Previous Sprints (if any)

If `sprint_num > 1`, read previous sprint plans to see what has already been built:
- `sprints/v1/sprint-plan.md`
- `sprints/v2/sprint-plan.md`
- etc.

This helps avoid duplicating features and ensures continuity.

### Step 4: Ask User to Select Features

Present the user with all features from the PRD and ask which ones they want to build in this sprint.

Guidance to provide:
- **Sprint scope:** Typically 2-4 weeks of work
- **Start small:** For Sprint v1, recommend starting with core/foundational features
- **Dependencies:** Some features may depend on others (e.g., need auth before user profiles)
- **Incremental value:** Each sprint should deliver working, valuable features

Example interaction:
```
I found 15 features in your PRD:

Core Features (Critical):
1. User Authentication (JWT-based login)
2. User Profile Management
3. Dashboard

Main Features (High Priority):
4. Data Import (CSV)
5. Data Export (JSON, CSV)
6. Search Functionality
7. Analytics Dashboard

Additional Features (Medium Priority):
8. Email Notifications
9. Webhooks
10. API Rate Limiting
... etc

For Sprint v1, I recommend starting with foundational features.

Which features would you like to build in Sprint v1?
(You can select multiple, e.g., "1, 2, 3" or "User Auth, Dashboard")
```

### Step 5: Define Sprint Goal

Based on selected features, help the user define a clear sprint goal.

Example:
- "Enable users to authenticate and view their dashboard"
- "Build core data import/export functionality"
- "Implement basic search and analytics"

### Step 6: Identify Dependencies

For the selected features, identify any dependencies:
- Which features must be built first?
- Are there any technical prerequisites?
- Any blockers or unknowns?

### Step 7: Create Sprint Plan Document

Create `sprints/v{sprint_num}/sprint-plan.md`:

```markdown
# Sprint v{N} Plan

**Sprint Number:** {N}

**Duration:** {Start Date} - {End Date} (typically 2 weeks)

**Sprint Goal:** {Clear, concise goal statement}

---

## Features in This Sprint

### Feature 1: {Feature Name}
- **From PRD:** {Link or reference to PRD section}
- **Priority:** Critical / High / Medium
- **Description:** {What this feature does}
- **User Story:** As a [user], I want to [action] so that [benefit]
- **Success Criteria:**
  - [ ] {Specific, measurable criterion 1}
  - [ ] {Specific, measurable criterion 2}
  - [ ] {Specific, measurable criterion 3}

### Feature 2: {Feature Name}
[Same structure]

### Feature 3: {Feature Name}
[Same structure]

---

## Dependencies

- **Feature 1** must be completed before **Feature 2** (because...)
- **Feature 3** requires **external API** setup

---

## Out of Scope for This Sprint

[List features that are NOT being built in this sprint]

---

## Risks & Concerns

- [ ] {Any potential blocker or risk}
- [ ] {Technical unknowns}
- [ ] {External dependencies}

---

## Definition of Done

A feature is considered "done" when:
- [ ] Code implemented and working
- [ ] Unit tests written and passing (80%+ coverage)
- [ ] Contract tests written (for APIs)
- [ ] Integration tests written
- [ ] Code reviewed and approved
- [ ] Documentation updated
- [ ] Deployed to staging (if applicable)

---

## Estimated Effort

- Feature 1: {X days}
- Feature 2: {Y days}
- Feature 3: {Z days}

**Total:** {Total days}

---

## Notes

[Any additional context or decisions made during planning]
```

### Step 8: Create Sprint Directory

Create the sprint directory:
```bash
mkdir -p sprints/v{sprint_num}
```

### Step 9: Increment Sprint Counter (Optional)

Ask the user if they want to increment the sprint counter in `.prodkit/config.yml` for next time.

If yes, update `sprints.current` to `{sprint_num + 1}`.

### Step 10: Validate Sprint Plan Created

Run validation checks to ensure sprint plan was created successfully:

Display validation in progress:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  VALIDATING SPRINT PLAN
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**Check 1: Sprint Plan File Exists**
```bash
SPRINT_NUM=$(grep "current:" .prodkit/config.yml | sed 's/.*current: //')
SPRINT_PLAN="sprints/v${SPRINT_NUM}/sprint-plan.md"

if [ ! -f "$SPRINT_PLAN" ]; then
    echo "❌ Validation failed: Sprint plan not found"
    echo "Expected: $SPRINT_PLAN"
    exit 1
fi

echo "✓ Sprint plan file exists"
```

**Check 2: Sprint Plan Has Content**
```bash
if [ ! -s "$SPRINT_PLAN" ]; then
    echo "❌ Validation failed: Sprint plan is empty"
    exit 1
fi

echo "✓ Sprint plan has content"
```

**Check 3: Sprint Goal Defined**
```bash
if ! grep -q "## Sprint Goal" "$SPRINT_PLAN" && ! grep -q "\*\*Sprint Goal:\*\*" "$SPRINT_PLAN"; then
    echo "❌ Validation failed: Sprint goal not defined"
    echo "Sprint plan must have a clear sprint goal"
    exit 1
fi

echo "✓ Sprint goal defined"
```

**Check 4: Features Selected**
```bash
# Count features in the plan
FEATURE_COUNT=$(grep -c "^### Feature" "$SPRINT_PLAN" || echo "0")

if [ "$FEATURE_COUNT" -lt 1 ]; then
    echo "❌ Validation failed: No features selected for sprint"
    echo "At least one feature must be included in the sprint"
    exit 1
fi

echo "✓ $FEATURE_COUNT feature(s) selected for sprint"
```

**Check 5: PRD Exists (prerequisite)**
```bash
if [ ! -f "product/prd.md" ]; then
    echo "⚠️  Warning: PRD not found"
    echo "Run /prodkit.prd first to define product features"
else
    echo "✓ PRD exists"
fi
```

**Check 6: Sprint Directory Created**
```bash
SPRINT_DIR="sprints/v${SPRINT_NUM}"

if [ ! -d "$SPRINT_DIR" ]; then
    echo "⚠️  Warning: Sprint directory not found: $SPRINT_DIR"
else
    echo "✓ Sprint directory created"
fi
```

Display validation complete:
```
✓ All validation checks passed

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Step 11: Confirm Completion

Inform the user:
- ✓ Sprint v{N} plan created at `sprints/v{N}/sprint-plan.md`
- ✓ Sprint plan validated with {X} features selected
- Features selected: {list}
- Sprint goal: {goal}
- Next step: Run `/prodkit.sprint-tech` to create technical design for these features

## Important Notes

- **Start small for Sprint v1:** Build foundation first (auth, core data models)
- **Consider dependencies:** Some features can't be built until others are complete
- **Realistic scope:** Better to complete 3 features well than 10 features poorly
- **Sprint goal should be clear:** Anyone should understand what you're building

## Example Sprint v1 Features

Good choices for Sprint v1:
- User authentication and registration
- Core data models
- Basic CRUD operations
- Database setup and migrations

Bad choices for Sprint v1:
- Advanced analytics (needs data first)
- Email notifications (secondary feature)
- Third-party integrations (added complexity)

## Output

After this command, the user should have:
- `sprints/v{N}/sprint-plan.md` with selected features
- Clear sprint goal
- Understanding of what will be built
- Ready to move to technical design phase
