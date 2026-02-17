---
description: Create Product Requirements Document (PRD)
---

You are helping the user create a comprehensive Product Requirements Document (PRD) for their product.

## Context

The PRD is the foundational document that defines WHAT the product is and WHY it exists. It should describe the product vision, all features, user stories, and success metrics.

This is a PRODUCT-LEVEL document (not sprint-level). It contains ALL features for the entire product, not just what will be built in the first sprint.

## Instructions

1. **Gather Product Information**

   Ask the user the following questions interactively:

   a. **Product Overview:**
      - What is the name of the product?
      - What problem does it solve?
      - Who are the target users?
      - What is the core value proposition?

   b. **Product Vision:**
      - What is the long-term vision for this product?
      - What does success look like in 6 months? 1 year?

   c. **Features & Capabilities:**
      - What are ALL the features you envision for this product?
      - List them comprehensively (even if some won't be built in Sprint 1)
      - For each feature, understand: What does it do? Why is it needed?

   d. **User Stories:**
      - Who are the different types of users?
      - What are their key workflows and use cases?

   e. **Success Metrics:**
      - How will you measure if the product is successful?
      - What are the key performance indicators (KPIs)?

   f. **Scope & Constraints:**
      - What is explicitly OUT of scope?
      - Any technical, time, or resource constraints?

2. **Create the PRD Document**

   Write a comprehensive PRD at: `product/prd.md`

   Use this structure:

   ```markdown
   # Product Requirements Document

   ## Product Overview

   **Product Name:** [Name]

   **Version:** 1.0

   **Date:** [Current Date]

   **Author:** Product Team

   ## Problem Statement

   [Describe the problem this product solves]

   ## Target Users

   [Describe who will use this product]

   ## Product Vision

   [Long-term vision and goals]

   ## Value Proposition

   [Why users should use this product]

   ## Success Metrics

   [KPIs and how success will be measured]

   ## Features

   ### Feature 1: [Name]
   - **Description:** [What it does]
   - **User Story:** As a [user type], I want to [action] so that [benefit]
   - **Priority:** [Critical / High / Medium / Low]
   - **Dependencies:** [Any dependencies on other features]

   ### Feature 2: [Name]
   [Same structure]

   [Continue for all features...]

   ## User Workflows

   ### Workflow 1: [Name]
   [Describe the user journey]

   ### Workflow 2: [Name]
   [Describe the user journey]

   ## Out of Scope

   [What will NOT be included in this product]

   ## Constraints

   [Technical, time, budget, or other constraints]

   ## Roadmap (High-Level)

   - **Phase 1 (Sprint v1-v2):** [Core features]
   - **Phase 2 (Sprint v3-v4):** [Enhanced features]
   - **Phase 3 (Sprint v5+):** [Advanced features]

   ## Appendix

   [Any additional context, research, or references]
   ```

3. **Update Configuration**

   After creating the PRD, update `.prodkit/config.yml`:
   - Set `project.name` to the product name
   - Set `project.description` to a brief description

4. **Validate PRD Created**

   Run validation checks to ensure PRD was created successfully:

   Display validation in progress:
   ```
   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     VALIDATING PRD
   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   ```

   **Check 1: PRD File Exists**
   ```bash
   PRD_FILE="product/prd.md"

   if [ ! -f "$PRD_FILE" ]; then
       echo "❌ Validation failed: PRD file not found"
       echo "Expected: $PRD_FILE"
       exit 1
   fi

   echo "✓ PRD file exists"
   ```

   **Check 2: PRD Has Content**
   ```bash
   if [ ! -s "$PRD_FILE" ]; then
       echo "❌ Validation failed: PRD file is empty"
       exit 1
   fi

   echo "✓ PRD has content"
   ```

   **Check 3: PRD Has Required Sections**
   ```bash
   MISSING_SECTIONS=()

   if ! grep -q "## Product Overview" "$PRD_FILE"; then MISSING_SECTIONS+=("Product Overview"); fi
   if ! grep -q "## Features" "$PRD_FILE"; then MISSING_SECTIONS+=("Features"); fi
   if ! grep -q "## User Stories" "$PRD_FILE"; then MISSING_SECTIONS+=("User Stories"); fi
   if ! grep -q "## Success Metrics" "$PRD_FILE"; then MISSING_SECTIONS+=("Success Metrics"); fi

   if [ ${#MISSING_SECTIONS[@]} -gt 0 ]; then
       echo "❌ Validation failed: PRD missing required sections: ${MISSING_SECTIONS[*]}"
       exit 1
   fi

   echo "✓ PRD has all required sections"
   ```

   **Check 4: Features Defined**
   ```bash
   FEATURE_COUNT=$(grep -c "^### Feature" "$PRD_FILE" || echo "0")

   if [ "$FEATURE_COUNT" -lt 1 ]; then
       echo "⚠️  Warning: No features defined in PRD"
       echo "At least one feature should be defined under ## Features section"
   else
       echo "✓ PRD has $FEATURE_COUNT feature(s) defined"
   fi
   ```

   **Check 5: Config Updated**
   ```bash
   if [ ! -f ".prodkit/config.yml" ]; then
       echo "⚠️  Warning: Config file not found"
   else
       PROJECT_NAME=$(grep "name:" .prodkit/config.yml | head -1 | sed 's/.*name: "\(.*\)".*/\1/' | sed 's/.*name: //' | tr -d '"')

       if [ "$PROJECT_NAME" = "my-project" ] || [ -z "$PROJECT_NAME" ]; then
           echo "⚠️  Warning: Project name not updated in config.yml"
       else
           echo "✓ Config updated with project name: $PROJECT_NAME"
       fi
   fi
   ```

   Display validation complete:
   ```
   ✓ All validation checks passed

   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   ```

5. **Confirm Completion**

   After creating and validating the PRD, inform the user:
   - ✓ PRD created at `product/prd.md`
   - ✓ PRD validated with {X} features defined
   - Total number of features defined
   - Next step: Run `/prodkit.product-arch` to define the technical architecture

## Important Notes

- The PRD should be focused on WHAT and WHY, not HOW (that's for tech docs)
- Be comprehensive - include all features, even if they won't be built immediately
- Use clear, non-technical language that anyone can understand
- Include user stories to make features concrete and user-focused
- Prioritize features to help with sprint planning later

## Example Output

After running this command, the user should have:
- `product/prd.md` - Complete PRD with all features
- Updated `.prodkit/config.yml` with project details
- Clear understanding of their product vision and features
