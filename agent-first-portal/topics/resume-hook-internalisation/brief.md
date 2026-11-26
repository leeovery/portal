# Resume Hook Internalisation

Today the user registers a hook themselves to wire up Portal resume commands. This topic makes that registration internal and managed by Portal, likely reusing the same versioned, idempotent, non-destructive hook-install machinery introduced for agent-awareness hooks but applied to the existing resume hooks. Kept conceptually separate from the agent-awareness hooks (different concept, sibling not merged). Raised in conversation and confirmed in scope; the imported plan keeps the resume engine itself as-is and does not cover auto-registration.
