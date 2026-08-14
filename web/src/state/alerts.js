export const AlertsStore = (() => {
  async function refresh() {
    try {
      const response = await AlertsAPI.list();
      const items = response.alerts || [];
      Store.set({ alerts: { items, badge: items.filter((a) => a.severity !== 'information' && a.status !== 'resolved').length } });
    } catch (_) {
      // Alerts should not interrupt core operations UI.
    }
  }
  return { refresh };
})();
