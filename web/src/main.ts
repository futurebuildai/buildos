/**
 * App entry point. Boots the session (silent refresh if possible, else
 * anonymous), then starts the router. Auth/capability stores register their
 * side-effects (interceptor wiring, BroadcastChannel) on import.
 */
import './components/app/fb-app.js';
import './components/pages/index.js';
import { initSession } from './state/authStore.js';
import { startRouter } from './router.js';
import { initObservability } from './obs/sentry.js';

async function bootstrap(): Promise<void> {
  initObservability();
  await initSession();
  startRouter();
}

void bootstrap();
