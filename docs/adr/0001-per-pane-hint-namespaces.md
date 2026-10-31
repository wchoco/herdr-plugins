# yubi Hints are closed within each Visible Pane

When yubi copies from every Visible Pane, not only the Target Pane, each Neighbour Pane gets a Pane Label fixed when yubi opens, and its Hints are that label followed by Hints assigned within the pane alone. The Target Pane has no label and gets its Hints from the alphabet minus the letters used as labels. Labels are taken from the end of `hint.alphabet`, so the easiest keys stay with the Target Pane, the pane copied from most often. Because no pane's Hints depend on another pane's Matches, Neighbour Panes can be read in the background and drawn as they arrive without any Hint already on screen changing, and the same layout gives the same Hints every time.

## Considered Options

- **One prefix-free Hint set across all panes, shortest Hints to the Target Pane.** This gives the shortest Hints overall, but the set depends on the total Match count: a pane read late changes Hints already on screen, so yubi would have to wait for every pane before drawing.
- **Hints taken from what is left, in the order panes arrive, never reassigned.** Shown Hints stay put, but they differ from one launch to the next, and a busy Target Pane can leave no Hints for the Neighbour Panes.

## Consequences

- A Neighbour Pane's Hints are two or three letters long.
- Identical Matches share a Hint only within one pane.
- Line Mode uses the same Pane Labels in front of Line Numbers (`n3-5`, with `n` the first label of the default alphabet). A Line Number with no label means the Target Pane, so existing input keeps its meaning.
