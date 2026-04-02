---
description: Design the frontend state architecture. Define state tree structure, action/mutation/effect taxonomy, persistence strategy, optimistic update patterns, and offline/online sync rules. Approach is chosen based on TECH_STACK.md.
invoked_from:
  - workflows/antigravity/07_architecture_spec.md
  - workflows/antigravity/feature_iteration.md
produces:
  - State tree structure and categorization
  - Action/mutation/effect taxonomy
  - Persistence strategy (which state survives refresh/navigation)
  - Optimistic update patterns
  - Offline/online synchronization rules
  - State management library configuration
browser_usage: None (relies on TECH_STACK.md and architecture inputs)
---

# Skill: State Management Design

Design how the frontend application manages data. State management is the nervous system of the UI — get it wrong and the application becomes a tangle of stale data, race conditions, and unpredictable behavior. Get it right and the UI becomes predictable, testable, and fast.

---

## Prerequisites

Before invoking this skill, ensure the following exist:

- `TECH_STACK.md` — for frontend framework, state management library, data fetching approach
- `ARCHITECTURE.md` (partial) — for API contract and data model
- `INFORMATION_ARCHITECTURE.md` — for understanding which pages share data
- `api_contract_design.md` output (if already produced) — for API shapes

---

## Step 1: Categorize State

All state in the application falls into one of these categories:

### State Categories

| Category | Definition | Lifetime | Shared Across Pages | Example |
|----------|-----------|----------|--------------------|---------|
| **Server State** | Data that originates from the backend | Until invalidated/refetched | Yes | User profile, list of items, settings |
| **Client State (Global)** | App-wide UI state not from the server | Session or persistent | Yes | Theme preference, sidebar collapsed, feature flags |
| **Client State (Local)** | Component-scoped UI state | Component lifetime | No | Form input values, dropdown open/closed, hover state |
| **URL State** | State encoded in the URL | Navigation lifetime | Yes (via URL) | Current page, filters, search query, sort order |
| **Form State** | In-progress form data | Until submit or abandon | No (usually) | Draft form values, validation errors, dirty tracking |
| **Derived State** | Computed from other state (never stored directly) | Recomputed on dependency change | Depends on source | Filtered lists, computed totals, formatted dates |

### State Inventory

For each piece of state in the application:

| State | Category | Source | Scope | Persistence | Invalidation |
|-------|----------|--------|-------|-------------|-------------|
| Current user | Server | GET /me | Global | Session storage | On logout / token refresh |
| [Entity] list | Server | GET /[entities] | Page | Cache (stale-while-revalidate) | On mutation / time-based |
| Theme | Client (Global) | User preference | Global | localStorage | On user change |
| Sidebar open | Client (Global) | UI interaction | Global | localStorage | On user toggle |
| Modal open | Client (Local) | UI interaction | Component | None (ephemeral) | On close |
| Search query | URL | URL params | Page | URL | On navigation |

---

## Step 2: Choose State Management Approach

Based on TECH_STACK.md, define the approach for each state category:

### Approach by Framework

**React:**
| Category | Recommended Approach | Library/Pattern |
|----------|---------------------|-----------------|
| Server State | Query cache | TanStack Query / SWR / Apollo Client |
| Client (Global) | Context + reducer or external store | Zustand / Jotai / Redux Toolkit |
| Client (Local) | Component state | `useState` / `useReducer` |
| URL State | Router state | React Router / Next.js router |
| Form State | Form library | React Hook Form / Formik |
| Derived State | Selectors/memos | `useMemo` / store selectors |

**Vue:**
| Category | Recommended Approach | Library/Pattern |
|----------|---------------------|-----------------|
| Server State | Query cache | TanStack Query for Vue / custom composables |
| Client (Global) | Reactive store | Pinia |
| Client (Local) | Component state | `ref` / `reactive` |
| URL State | Router state | Vue Router |
| Form State | Form library | VeeValidate / FormKit |
| Derived State | Computed properties | `computed` / store getters |

**Svelte:**
| Category | Recommended Approach | Library/Pattern |
|----------|---------------------|-----------------|
| Server State | Query cache or stores | TanStack Query for Svelte / custom stores |
| Client (Global) | Stores | Svelte stores (writable/readable) |
| Client (Local) | Component variables | `let` declarations |
| URL State | Router/page state | SvelteKit `$page` store |
| Form State | Component state + validation | Superforms / custom |
| Derived State | Derived stores | `derived` stores |

**Lit / Web Components:**
| Category | Recommended Approach | Library/Pattern |
|----------|---------------------|-----------------|
| Server State | Custom controller or service | Fetch + event-based updates |
| Client (Global) | Shared reactive controller | Lit reactive controllers / custom events |
| Client (Local) | Reactive properties | `@state()` decorator |
| URL State | Router state | Custom router / URL API |
| Form State | Component state | Reactive properties |
| Derived State | Getters | Computed getters on element class |

Select the approach matching TECH_STACK.md and document it.

---

## Step 3: State Tree Structure

Define the shape of the global state:

```typescript
// Example state tree (adapt to chosen library)
interface AppState {
  // Server state (managed by query cache)
  // These are not in the global store — they live in the query cache
  // Documented here for completeness

  // Client global state
  ui: {
    theme: 'light' | 'dark';
    sidebarCollapsed: boolean;
    activeModal: string | null;
  };

  auth: {
    isAuthenticated: boolean;
    user: User | null;
    permissions: string[];
  };

  // Feature-specific state (if needed beyond server cache)
  [feature]: {
    // Feature-specific client state
  };
}
```

### Naming Conventions

| Convention | Rule | Example |
|-----------|------|---------|
| Store/slice names | camelCase, noun | `auth`, `ui`, `notifications` |
| Actions | camelCase, verb+noun | `setTheme`, `toggleSidebar`, `clearNotifications` |
| Selectors | camelCase, `select` prefix or descriptive | `selectCurrentUser`, `isAuthenticated` |
| Async actions | camelCase, verb+noun | `fetchUser`, `createOrder` |
| Mutation/reducer names | camelCase, past tense or imperative | `userLoaded`, `setTheme` |

---

## Step 4: Server State Management

### Data Fetching Strategy

| Pattern | When to Use | Configuration |
|---------|-------------|---------------|
| Fetch on mount | Page-level data, always fresh | `staleTime: 0` |
| Stale-while-revalidate | Data that changes infrequently | `staleTime: 60_000` (or per-resource) |
| Prefetch on hover/focus | Predictable navigation | Prefetch queries on link hover |
| Infinite query | Paginated lists with scroll | Cursor-based, append pages |
| Polling | Near-real-time updates without WebSocket | `refetchInterval: 5_000` |
| WebSocket subscription | Real-time data | Subscribe on mount, update cache on message |

### Query Key Strategy

Define a consistent query key structure:

```typescript
// Query key factory pattern
const queryKeys = {
  users: {
    all: ['users'] as const,
    lists: () => [...queryKeys.users.all, 'list'] as const,
    list: (filters: UserFilters) => [...queryKeys.users.lists(), filters] as const,
    details: () => [...queryKeys.users.all, 'detail'] as const,
    detail: (id: string) => [...queryKeys.users.details(), id] as const,
  },
  // Repeat for each resource
};
```

### Cache Invalidation Rules

| Trigger | Invalidation Scope | Method |
|---------|-------------------|--------|
| Create [entity] | All list queries for [entity] | `invalidateQueries(['entities', 'list'])` |
| Update [entity] | Specific detail + all lists | `invalidateQueries(['entities'])` |
| Delete [entity] | Specific detail + all lists | `removeQueries(['entities', 'detail', id])` + invalidate lists |
| Bulk operation | All queries for [entity] | `invalidateQueries(['entities'])` |
| Logout | All queries | `queryClient.clear()` |

---

## Step 5: Action / Mutation / Effect Taxonomy

### Actions (User Intentions)

Actions represent what the user wants to do:

| Action | Trigger | State Changes | Side Effects |
|--------|---------|--------------|-------------|
| `login` | Login form submit | Set auth state, store token | API call, redirect |
| `toggleTheme` | Theme button click | Toggle `ui.theme` | Write to localStorage |
| `createEntity` | Create form submit | Optimistic insert | API call, invalidate cache |
| `deleteEntity` | Delete button confirm | Optimistic remove | API call, invalidate cache |
| `setFilter` | Filter control change | Update URL state | Trigger refetch |

### Mutations (State Changes)

Mutations are the atomic state changes caused by actions:

| Mutation | State Modified | Payload | Validation |
|----------|---------------|---------|------------|
| `setUser` | `auth.user` | `User` | Non-null |
| `clearUser` | `auth.user`, `auth.permissions` | None | N/A |
| `setTheme` | `ui.theme` | `'light' \| 'dark'` | Enum check |
| `setSidebarCollapsed` | `ui.sidebarCollapsed` | `boolean` | Type check |

### Effects (Side Effects)

Effects are async operations triggered by actions:

| Effect | Triggered By | API Call | On Success | On Failure |
|--------|-------------|----------|-----------|-----------|
| `fetchUser` | App mount, login | `GET /me` | `setUser` mutation | Redirect to login |
| `createEntity` | Create action | `POST /entities` | Invalidate list cache | Rollback optimistic update, show error |
| `updateEntity` | Update action | `PUT /entities/:id` | Invalidate caches | Rollback, show error |

---

## Step 6: Persistence Strategy

### What Persists and Where

| State | Storage | Serialization | TTL | Clear On |
|-------|---------|-------------|-----|----------|
| Auth token | httpOnly cookie (preferred) or secure storage | N/A (cookie) or encrypted | Token TTL | Logout |
| User preferences (theme, etc.) | localStorage | JSON | Indefinite | User reset |
| UI state (sidebar, etc.) | localStorage | JSON | Indefinite | User reset |
| Draft form data | sessionStorage or IndexedDB | JSON | Session | Submit or abandon |
| Server cache | Memory (query cache) | N/A | `staleTime` / `gcTime` | Page reload |
| URL state (filters, page) | URL params | URL encoding | Navigation | New navigation |

### Persistence Implementation

```typescript
// Example: Zustand with localStorage persistence
const useUIStore = create(
  persist(
    (set) => ({
      theme: 'light',
      sidebarCollapsed: false,
      setTheme: (theme) => set({ theme }),
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
    }),
    {
      name: 'ui-preferences',
      partialize: (state) => ({
        theme: state.theme,
        sidebarCollapsed: state.sidebarCollapsed,
      }),
    }
  )
);
```

### Hydration Strategy

On application load:
1. Read persisted state from storage
2. Validate persisted state shape (handle schema migrations for localStorage)
3. Merge with default state (persisted values override defaults)
4. Fetch fresh server state (auth check, user profile)
5. Application is ready once auth state is resolved

---

## Step 7: Optimistic Update Patterns

### When to Use Optimistic Updates

| Operation | Use Optimistic? | Rationale |
|-----------|----------------|-----------|
| Toggle (like, bookmark) | Yes | High confidence of success, instant feedback critical |
| Create item in list | Yes (if UI can generate temp ID) | Better UX for add operations |
| Update single field | Yes | Small change, high confidence |
| Delete item | Yes | Immediate visual feedback |
| Complex multi-step operation | No | Too many failure points |
| Financial transaction | No | Must confirm server success |

### Optimistic Update Flow

```
1. User triggers action
2. Immediately update local cache/state with expected result
3. Send request to server
4. If success: replace optimistic data with server response (if different)
5. If failure: rollback to previous state, show error
```

### Implementation Pattern

```typescript
// Example: TanStack Query optimistic update
const mutation = useMutation({
  mutationFn: updateEntity,
  onMutate: async (newData) => {
    // Cancel outgoing refetches
    await queryClient.cancelQueries({ queryKey: ['entities', newData.id] });

    // Snapshot previous value
    const previous = queryClient.getQueryData(['entities', newData.id]);

    // Optimistically update
    queryClient.setQueryData(['entities', newData.id], newData);

    // Return rollback context
    return { previous };
  },
  onError: (err, newData, context) => {
    // Rollback on error
    queryClient.setQueryData(['entities', newData.id], context.previous);
  },
  onSettled: () => {
    // Refetch to ensure consistency
    queryClient.invalidateQueries({ queryKey: ['entities'] });
  },
});
```

---

## Step 8: Offline/Online Sync Rules

Define behavior when the network connection is lost and restored.

### Offline Detection

```typescript
// Listen for online/offline events
window.addEventListener('online', handleOnline);
window.addEventListener('offline', handleOffline);

// Also use navigator.onLine as initial state
// Consider periodic fetch-based health checks for more reliable detection
```

### Offline Behavior by State Category

| Category | Offline Behavior |
|----------|-----------------|
| Server state (read) | Serve from cache if available, show stale indicator |
| Server state (write) | Queue mutations for later (if supported) or show offline message |
| Client state | Fully functional (no server dependency) |
| URL state | Fully functional |
| Form state | Fully functional, disable submit button with offline indicator |

### Mutation Queue (if offline write support is needed)

| Property | Value |
|----------|-------|
| Storage | IndexedDB (survives page reload) |
| Queue order | FIFO (first in, first out) |
| Conflict resolution | Last-write-wins or server-side conflict detection |
| Max queue size | [N] mutations |
| Queue TTL | [N] hours (discard stale mutations) |
| Retry on reconnect | Auto-flush queue with exponential backoff |

### Online Recovery Flow

```
1. Detect online event
2. Verify connectivity (fetch health check endpoint)
3. Flush mutation queue in order (with retry on failure)
4. Invalidate all stale server state queries
5. Remove stale indicator from UI
6. Notify user of sync status ("All changes saved" or "X changes synced")
```

---

## Step 9: Cross-Reference Validation

Before finalizing, verify:

- [ ] Every piece of data in the UI is categorized (server, client global, client local, URL, form, derived)
- [ ] State management approach is consistent with TECH_STACK.md framework and libraries
- [ ] Server state caching strategy is defined for every API endpoint
- [ ] Cache invalidation rules cover all mutation scenarios
- [ ] Persistence covers user preferences and draft data
- [ ] Optimistic updates are defined for high-frequency interactions
- [ ] Offline behavior is defined (even if "show offline message" is the only strategy)
- [ ] No derived state is stored (always computed from source state)

---

## Output

The final output feeds into the Frontend Architecture section of `ARCHITECTURE.md`. The state management design serves as the implementation guide for the frontend developer setting up stores, query configurations, and data flow patterns.
