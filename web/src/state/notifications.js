import { Store } from '../state.js';

export const Notifications = { toast: (message, tone = 'info') => Store.toast(message, tone) };
