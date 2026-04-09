# Persist Dashboard Power History Across Navigation

## Problem

Dashboard page maintains a `powerHistory` array (last 60 data points, ~3 minutes) in component-local state (`useState`). When the user navigates to PlantDetail and returns, Dashboard unmounts and all chart history is lost. This makes it impossible to observe total power trends (rising/falling) across navigation.

## Requirements

- Chart history (last 60 data points) must survive navigation between Dashboard and PlantDetail
- Data only needs to persist within the browser session (tab close = reset is acceptable)
- Keep the data window at 60 points (current behavior), no change needed
- Minimal code changes, no new dependencies

## Chosen Approach: Lift State to App

Move `powerHistory` state and its accumulation logic from `Dashboard.tsx` up to `App.tsx`. Since `App` never unmounts during the session, the history persists across route changes.

### Alternatives Considered

1. **Layout route with `<Outlet>`** — keeps Dashboard mounted while showing PlantDetail. Rejected: changes page structure and UX, over-engineered for this need.
2. **Module-level variable** — store history outside React. Rejected: fragile with HMR, not idiomatic React.

## Design

### Changes to `App.tsx`

1. Add `powerHistory` state: `useState<{ time: string; watt: number }[]>([])`
2. Compute `totalWatt` from `plants` — direct calculation, no `useMemo` (plant count is small, `reduce` is trivial)
3. Compute `lastSeen` from `plants` — direct calculation, no `useMemo` (same reasoning)
4. Add `useEffect` triggered by `lastSeen` that appends `{ time, watt }` to history, capping at 60 entries
   - `lastSeen` is used as a polling tick — it changes on every poll cycle even when `totalWatt` stays constant
   - `totalWatt` is intentionally read from the current render closure, not listed as a dependency (with `eslint-disable-next-line react-hooks/exhaustive-deps` and an inline comment explaining why)
5. Pass `powerHistory` as a prop to `<Dashboard>`

### Changes to `Dashboard.tsx`

1. Add `powerHistory` to `DashboardProps` interface
2. Remove local `powerHistory` state (`useState` on line 39)
3. Remove the `useEffect` that accumulates history (lines 41-47)
4. Remove `lastSeen` computation (lines 34-37) — moved to App
5. Remove `totalWatt` `useMemo` — replaced by direct calculation in App, passed down or kept as direct calculation in Dashboard for display
6. `PowerChart` continues to receive `data={powerHistory}` from props instead of local state

### Unchanged

- `usePlants` hook — no changes
- `PowerChart` component — no changes
- `PlantDetail` page — no changes
- Route structure — no changes
- Data polling intervals — no changes

## Data Flow (After)

```
App.tsx
  ├── plants (from usePlants)
  ├── totalWatt (direct calculation from plants, no memo)
  ├── lastSeen (direct calculation from plants, no memo)
  ├── powerHistory[] (useState, accumulated via useEffect on lastSeen)
  │
  ├── Route "/" → Dashboard
  │     └── receives powerHistory as prop → renders PowerChart
  │
  └── Route "/plants/:plantId" → PlantDetail
        (powerHistory continues accumulating in App while user is here)
```

## Future Considerations

- If additional routes or components need access to `powerHistory`, consider extracting the accumulation logic into a dedicated `PowerHistoryProvider` context to avoid prop drilling.

## Scope

- 2 files modified: `App.tsx`, `Dashboard.tsx`
- 0 new files
- 0 new dependencies
