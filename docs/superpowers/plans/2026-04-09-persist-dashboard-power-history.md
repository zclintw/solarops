# Persist Dashboard Power History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make powerHistory survive navigation between Dashboard and PlantDetail by lifting state from Dashboard to App.

**Architecture:** Move `powerHistory` state and its accumulation `useEffect` from `Dashboard.tsx` to `App.tsx`. Dashboard receives `powerHistory` as a prop. App never unmounts, so history persists across route changes.

**Tech Stack:** React 19, React Router 7, TypeScript

**Note:** Frontend has no test infrastructure (no vitest/jest). Verification is manual via dev server.

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `frontend/src/App.tsx` | Modify | Add `powerHistory` state, `totalWatt`/`lastSeen` calculations, accumulation `useEffect`, pass prop to Dashboard |
| `frontend/src/pages/Dashboard.tsx` | Modify | Accept `powerHistory` prop, remove local state/effect/lastSeen/totalWatt-memo |

---

### Task 1: Add power history accumulation to App.tsx

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Add imports**

Add `useState`, `useEffect` to the existing import from `react`:

```tsx
import { useCallback, useState, useEffect } from "react";
```

- [ ] **Step 2: Add totalWatt, lastSeen, powerHistory state and effect**

After the `const { send } = useWebSocket(onMessage);` line (line 19), add:

```tsx
const plantEntries = Object.entries(plants);

const totalWatt = plantEntries.reduce(
  (sum, [, state]) => sum + (state.summary?.totalWatt || 0),
  0
);

// lastSeen changes on every poll cycle, used as a tick to append history
const lastSeen = Math.max(0, ...plantEntries.map(([, s]) => s.lastSeen));

const [powerHistory, setPowerHistory] = useState<
  { time: string; watt: number }[]
>([]);

useEffect(() => {
  if (plantEntries.length === 0) return;
  setPowerHistory((prev) => [
    ...prev.slice(-59),
    { time: new Date().toLocaleTimeString(), watt: Math.round(totalWatt) },
  ]);
  // totalWatt is read from current render closure, not a dependency.
  // lastSeen is the polling tick that triggers history accumulation.
  // eslint-disable-next-line react-hooks/exhaustive-deps
}, [lastSeen]);
```

- [ ] **Step 3: Pass powerHistory prop to Dashboard**

Change the Dashboard route element from:

```tsx
<Dashboard
  plants={plants}
  alerts={alerts}
  onRemovePlant={removePlant}
  onAcknowledgeAlert={acknowledgeAlert}
  onResolveAlert={resolveAlert}
/>
```

to:

```tsx
<Dashboard
  plants={plants}
  alerts={alerts}
  powerHistory={powerHistory}
  onRemovePlant={removePlant}
  onAcknowledgeAlert={acknowledgeAlert}
  onResolveAlert={resolveAlert}
/>
```

- [ ] **Step 4: Verify App.tsx compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: Type error about `powerHistory` not being in `DashboardProps` (this is correct — we fix it in Task 2)

- [ ] **Step 5: Commit work-in-progress**

```bash
git add frontend/src/App.tsx
git commit -m "feat(frontend): add powerHistory accumulation to App.tsx

Lift powerHistory state from Dashboard to App so it persists across
route navigation. Dashboard prop update follows in next commit."
```

---

### Task 2: Remove local history state from Dashboard.tsx

**Files:**
- Modify: `frontend/src/pages/Dashboard.tsx`

- [ ] **Step 1: Add powerHistory to DashboardProps**

Change the `DashboardProps` interface from:

```tsx
interface DashboardProps {
  plants: Record<string, PlantState>;
  alerts: Alert[];
  onRemovePlant: (id: string) => void;
  onAcknowledgeAlert: (id: string) => void;
  onResolveAlert: (id: string) => void;
}
```

to:

```tsx
interface DashboardProps {
  plants: Record<string, PlantState>;
  alerts: Alert[];
  powerHistory: { time: string; watt: number }[];
  onRemovePlant: (id: string) => void;
  onAcknowledgeAlert: (id: string) => void;
  onResolveAlert: (id: string) => void;
}
```

- [ ] **Step 2: Accept powerHistory in destructured props**

Change the function signature from:

```tsx
export function Dashboard({
  plants,
  alerts,
  onRemovePlant,
  onAcknowledgeAlert,
  onResolveAlert,
}: DashboardProps) {
```

to:

```tsx
export function Dashboard({
  plants,
  alerts,
  powerHistory,
  onRemovePlant,
  onAcknowledgeAlert,
  onResolveAlert,
}: DashboardProps) {
```

- [ ] **Step 3: Remove local powerHistory state and accumulation effect**

Delete these lines (currently lines 33-47):

```tsx
  // lastSeen changes on every poll even when totalWatt stays constant
  const lastSeen = useMemo(
    () => Math.max(0, ...plantEntries.map(([, s]) => s.lastSeen)),
    [plantEntries]
  );

  const [powerHistory, setPowerHistory] = useState<{ time: string; watt: number }[]>([]);

  useEffect(() => {
    if (plantEntries.length === 0) return;
    setPowerHistory((prev) => [
      ...prev.slice(-59),
      { time: new Date().toLocaleTimeString(), watt: Math.round(totalWatt) },
    ]);
  }, [lastSeen]); // eslint-disable-line react-hooks/exhaustive-deps
```

- [ ] **Step 4: Remove totalWatt useMemo, replace with direct calculation**

Change:

```tsx
  const totalWatt = useMemo(
    () =>
      plantEntries.reduce(
        (sum, [, state]) => sum + (state.summary?.totalWatt || 0),
        0
      ),
    [plantEntries]
  );
```

to:

```tsx
  const totalWatt = plantEntries.reduce(
    (sum, [, state]) => sum + (state.summary?.totalWatt || 0),
    0
  );
```

- [ ] **Step 5: Clean up unused imports**

Change:

```tsx
import { useMemo, useState, useEffect } from "react";
```

to:

```tsx
import { useMemo } from "react";
```

Wait — `useMemo` is also no longer used (we removed the `totalWatt` memo and `lastSeen` memo). Remove it entirely. If no other React hooks remain, change to:

```tsx
import type { PlantState, Alert } from "../types";
```

Remove the entire `react` import line since no hooks are used in Dashboard anymore.

- [ ] **Step 6: Verify the full project compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add frontend/src/pages/Dashboard.tsx
git commit -m "feat(frontend): receive powerHistory as prop in Dashboard

Remove local powerHistory state, lastSeen useMemo, and totalWatt
useMemo from Dashboard. History is now accumulated in App.tsx and
passed down as a prop, surviving route navigation."
```

---

### Task 3: Manual verification

- [ ] **Step 1: Start the dev server**

Run: `cd frontend && npm run dev`

- [ ] **Step 2: Verify chart renders on Dashboard**

1. Open the app in browser
2. Wait ~30 seconds for several data points to accumulate
3. Confirm the "Total Power Output" chart shows a line with multiple points

- [ ] **Step 3: Verify history survives navigation**

1. Note the number of data points on the chart
2. Click on any plant card to navigate to PlantDetail
3. Wait ~10 seconds (history should keep accumulating in App)
4. Navigate back to Dashboard (browser back or click SolarOps header)
5. Confirm the chart still has all previous data points plus new ones accumulated while on PlantDetail

- [ ] **Step 4: Verify 60-point cap**

1. Leave Dashboard open for >3 minutes
2. Confirm chart shows at most 60 data points (oldest points are dropped)

- [ ] **Step 5: Final commit (if any adjustments were needed)**

If no adjustments needed, skip this step.
