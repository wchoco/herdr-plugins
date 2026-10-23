# herdr-plugins

Plugins that extend herdr, the terminal workspace manager for AI coding agents.

## yubi

**Target Pane**:
The herdr pane whose visible screen content yubi copies from.
_Avoid_: source pane, current pane

**Match**:
A string on the Target Pane's visible screen that fits one of yubi's patterns and is a copy candidate.
_Avoid_: candidate, token, hit

**Pattern**:
A rule that finds Matches on the Target Pane's visible screen; it is either a Built-in Pattern or a Custom Pattern.
_Avoid_: regex, rule, matcher

**Built-in Pattern**:
A Pattern that ships with yubi and can be disabled by name.
_Avoid_: default pattern

**Custom Pattern**:
A Pattern the user defines; it takes priority over every Built-in Pattern.
_Avoid_: user pattern, extra pattern

**Hint**:
A short label drawn over a Match; typing it copies that Match.
_Avoid_: tag, key, label

**Line Number**:
The number of a line on the Target Pane's visible screen, counted upward from 1 at the last non-blank line yubi displays.
_Avoid_: row, index, line hint

**Hint Mode**:
The yubi mode that shows a Hint on each Match.

**Line Mode**:
The yubi mode that shows a Line Number on each line.

**Joined Copy**:
A Line Mode copy that joins the selected lines without line breaks, producing a single line.
_Avoid_: one-line copy, flatten

**Paste Copy**:
A copy that also pastes the copied text into the Target Pane.
_Avoid_: copy-and-paste, upcase action, insert
