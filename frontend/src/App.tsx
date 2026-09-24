import { Route, Router, type RouteSectionProps } from '@solidjs/router';
import { onMount } from 'solid-js';
import NavBar from './components/NavBar';
import { withSession } from './components/RequireSession';
import ActivityMapPage from './pages/ActivityMapPage';
import ActivityPage from './pages/ActivityPage';
import FeedPage from './pages/FeedPage';
import LoginPage from './pages/LoginPage';
import NotFoundPage from './pages/NotFoundPage';
import NotificationsPage from './pages/NotificationsPage';
import ProfilePage from './pages/ProfilePage';
import ProfileStatsPage from './pages/ProfileStatsPage';
import RegisterPage from './pages/RegisterPage';
import ResetPage from './pages/ResetPage';
import SettingsPage from './pages/SettingsPage';
import UploadPage from './pages/UploadPage';
import VerifyPage from './pages/VerifyPage';
import { loadConfig, loadSession } from './lib/session';

function Layout(props: RouteSectionProps) {
  onMount(() => {
    void loadConfig();
    void loadSession();
  });

  return (
    <>
      <NavBar />
      <main>{props.children}</main>
    </>
  );
}

/** App wires every route under one layout. */
export default function App() {
  return (
    <Router root={Layout}>
      <Route path="/" component={withSession(FeedPage)} />
      <Route path="/login" component={LoginPage} />
      <Route path="/register" component={RegisterPage} />
      <Route path="/verify" component={VerifyPage} />
      <Route path="/reset" component={ResetPage} />
      <Route path="/upload" component={withSession(UploadPage)} />
      <Route path="/notifications" component={withSession(NotificationsPage)} />
      <Route path="/settings" component={withSession(SettingsPage)} />
      <Route path="/activities/:id/map" component={ActivityMapPage} />
      <Route path="/activities/:id" component={ActivityPage} />
      <Route path="/u/:username/stats" component={ProfileStatsPage} />
      <Route path="/u/:username" component={ProfilePage} />
      <Route path="*404" component={NotFoundPage} />
    </Router>
  );
}
