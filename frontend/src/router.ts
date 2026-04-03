/**
 * Hash-based SPA router for FutureBuild OS.
 * Uses #/path convention to avoid server-side routing requirements.
 * Supports path parameters via :param syntax.
 */

export interface Route {
  path: string;
  tag: string;
  title: string;
}

export interface RouteMatch {
  route: Route;
  params: Record<string, string>;
}

export const routes: Route[] = [
  { path: '/', tag: 'fb-dashboard-page', title: 'Dashboard' },
  { path: '/schedule', tag: 'fb-schedule-view', title: 'Schedule' },
  { path: '/pipeline', tag: 'fb-pipeline-view', title: 'Pipeline' },
  { path: '/pipeline/:id', tag: 'fb-prospect-detail-page', title: 'Prospect Detail' },
  { path: '/financials', tag: 'fb-financials-view', title: 'Financials' },
  { path: '/procurement', tag: 'fb-procurement-view', title: 'Procurement' },
  { path: '/fleet', tag: 'fb-fleet-view', title: 'Fleet' },
  { path: '/hr', tag: 'fb-hr-view', title: 'HR' },
  { path: '/briefing', tag: 'fb-briefing-view', title: 'Briefing' },
  { path: '/settings', tag: 'fb-settings-view', title: 'Settings' },
];

/**
 * Match the current hash against defined routes.
 * Supports :param path segments (e.g. /pipeline/:id).
 */
function matchRoute(hash: string): RouteMatch {
  const path = hash.replace('#', '') || '/';
  const pathSegments = path.split('/').filter(Boolean);

  for (const route of routes) {
    const routeSegments = route.path.split('/').filter(Boolean);

    if (pathSegments.length !== routeSegments.length) continue;

    const params: Record<string, string> = {};
    let matched = true;

    for (let i = 0; i < routeSegments.length; i++) {
      const routeSeg = routeSegments[i]!;
      const pathSeg = pathSegments[i]!;

      if (routeSeg.startsWith(':')) {
        params[routeSeg.slice(1)] = pathSeg;
      } else if (routeSeg !== pathSeg) {
        matched = false;
        break;
      }
    }

    if (matched) {
      return { route, params };
    }
  }

  // Default to dashboard
  return { route: routes[0]!, params: {} };
}

/**
 * Get the current route match from the URL hash.
 */
export function getCurrentRoute(): RouteMatch {
  return matchRoute(window.location.hash);
}

/**
 * Navigate to a path by updating the URL hash.
 * Dispatches a 'route-change' event on the window.
 */
export function navigateTo(path: string): void {
  window.location.hash = path;
}

/**
 * Custom event detail for route changes.
 */
export interface RouteChangeDetail {
  match: RouteMatch;
}

/**
 * Initialize the router — listens for hashchange and dispatches route-change events.
 */
export function initRouter(): void {
  const dispatch = () => {
    const match = getCurrentRoute();
    document.title = `${match.route.title} — FutureBuild OS`;
    window.dispatchEvent(
      new CustomEvent<RouteChangeDetail>('route-change', {
        detail: { match },
        bubbles: true,
      }),
    );
  };

  window.addEventListener('hashchange', dispatch);

  // Fire initial route
  dispatch();
}
