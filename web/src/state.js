export const Store = (() => {
  const state = {
    route: window.location.pathname,
    commandOpen: false,
    mobileNavOpen: false,
    toast: null,
    workspaceOpen: false,
    selectedDestination: null,
    selectedAlert: null,
    doctorResult: null,
    restoreReview: null,
    setupDraft: { provider: null, policy: null, engine: null },
    refreshToken: 0,
    jobFilter: 'all',
    connection: { status: 'disconnected', lastEventId: null, error: null },
    jobs: { byId: {}, orderedIds: [], activeIds: [], failedIds: [], completedIds: [], loading: false, error: null },
    alerts: { items: [], badge: 0 },
    confirmModal: null
  };
  const listeners = new Set();
  function set(patch) {
    Object.assign(state, patch);
    for (const listener of [...listeners]) listener(state);
  }
  function subscribe(listener) {
    listeners.add(listener);
    return () => listeners.delete(listener);
  }
  function navigate(route) {
    history.pushState({}, '', route);
    set({ route, mobileNavOpen: false, commandOpen: false, workspaceOpen: false });
    window.scrollTo({ top: 0, behavior: 'instant' });
  }
  function toast(message, tone = 'info') {
    set({ toast: { message, tone, id: Date.now() } });
    setTimeout(() => set({ toast: null }), 4200);
  }
  function refresh() { set({ refreshToken: state.refreshToken + 1 }); }
  function confirm(options) {
    return new Promise((resolve) => {
      const modalConfig = typeof options === 'string' ? { message: options } : options;
      set({
        confirmModal: {
          title: modalConfig.title || 'Confirm Action',
          message: modalConfig.message || 'Are you sure you want to proceed?',
          confirmLabel: modalConfig.confirmLabel || 'Confirm',
          cancelLabel: modalConfig.cancelLabel || 'Cancel',
          confirmTone: modalConfig.confirmTone || 'danger',
          resolve: (result) => {
            set({ confirmModal: null });
            resolve(result);
            if (result && modalConfig.onConfirm) modalConfig.onConfirm();
            if (!result && modalConfig.onCancel) modalConfig.onCancel();
          }
        }
      });
    });
  }
  window.addEventListener('popstate', () => set({ route: window.location.pathname, mobileNavOpen: false }));
  window.addEventListener('keydown', (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      set({ commandOpen: !state.commandOpen, mobileNavOpen: false });
    }
    if (event.key === 'Escape') set({ commandOpen: false, mobileNavOpen: false, workspaceOpen: false, confirmModal: null });
  });
  return { state, set, subscribe, navigate, toast, refresh, confirm };
})();

