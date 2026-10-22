# herdr-plugins

Plugins that extend herdr, the terminal workspace manager for AI coding agents.

## yubi

**Target Pane**:
The herdr pane whose visible screen content yubi copies from.
_Avoid_: source pane, current pane

**Match**:
A string on the Target Pane's visible screen that fits one of yubi's patterns and is a copy candidate.
_Avoid_: candidate, token, hit

**Hint**:
A short label drawn over a Match; typing it copies that Match.
_Avoid_: tag, key, label

**Line Number**:
The number of a line on the Target Pane's visible screen, counted upward from 1 at the last non-blank line.
_Avoid_: row, index, line hint

**Hint Mode**:
The yubi mode that shows a Hint on each Match.

**Line Mode**:
The yubi mode that shows a Line Number on each line.

**Joined Copy**:
A Line Mode copy that joins the selected lines without line breaks, producing a single line.
_Avoid_: one-line copy, flatten
