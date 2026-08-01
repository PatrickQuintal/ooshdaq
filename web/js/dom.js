function required(selector) {
  const element = document.querySelector(selector);
  if (!element) {
    throw new Error(`Missing browser element: ${selector}`);
  }
  return element;
}

export const elements = {
  app: required("#app"),
  skipLink: required(".skip-link"),
  mainContent: required("#main-content"),
  header: required("#app-header"),
  globalStatus: required("#global-status"),
  loginView: required("#login-view"),
  authenticatedViews: required("#authenticated-views"),
  mockUserList: required("#mock-user-list"),
  identityNote: required("#identity-note"),
  loginError: required("#login-error"),
  logoutButton: required("#logout-button"),
  headerAvatar: required("#header-avatar"),
  headerUserName: required("#header-user-name"),
  headerAuthMode: required("#header-auth-mode"),
  collectionNavLink: required("#collection-nav-link"),
  matchesNavBadge: required("#matches-nav-badge"),
  dashboardName: required("#dashboard-user-name"),
  summaryContent: required("#summary-content"),
  catalogFilters: required("#catalog-filters"),
  catalogSearch: required("#catalog-search"),
  catalogResults: required("#catalog-results"),
  catalogContent: required("#catalog-content"),
  catalogMore: required("#catalog-more"),
  collectionTitle: required("#collection-title"),
  collectionSubtitle: required("#collection-subtitle"),
  collectionSummary: required("#collection-summary"),
  collectionFilters: required("#collection-filters"),
  collectionSearch: required("#collection-search"),
  collectionUniverse: required("#collection-universe"),
  collectionState: required("#collection-state"),
  collectionResults: required("#collection-results"),
  collectionContent: required("#collection-content"),
  collectionMore: required("#collection-more"),
  matchesResults: required("#matches-results"),
  matchesContent: required("#matches-content"),
  matchesMore: required("#matches-more"),
  refreshMatches: required("#refresh-matches"),
  toastRegion: required("#toast-region"),
  announcer: required("#announcer"),
};
