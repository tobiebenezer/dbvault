import { Store } from '../../state.js';

export function JobToast(message, tone = 'info') { Store.toast(message, tone); }
