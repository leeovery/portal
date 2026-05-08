AGENT: standards

STATUS: clean
FINDINGS_COUNT: 0
SUMMARY: Implementation conforms to spec across read pipeline (single-fd, three-shape Tail contract, trailing-partial exclusion), page state machine (Update + View arms now matched after cycle 2), chrome (1-based ordinals, no liveness wording, no #W: prefix after cycle 1), keymap (preview owns Esc/Home/End/Tab/]/[ before viewport delegation), side-effect-free contract (hermetic test pins it), refresh-on-dismiss policy, and stateDir hidden behind the seam. Cycles 1 and 2 surfaced the actionable drift; cycle 3 finds nothing further.
