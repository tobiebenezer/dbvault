import { API } from '../api.js';

export const JobsAPI = {
  list: (params = {}) => API.jobs(params),
  get: (id) => API.job(id),
  detail: (id) => API.job(id),
  create: (payload = {}) => API.createJob(payload),
  createBackup: (payload = {}) => API.createJob({ job_type: 'backup', resource_id: payload.source_id, resource_name: payload.source_id, ...payload }),
  createDrill: (payload = {}) => API.createJob({ job_type: 'restore_drill', resource_id: payload.source_id, resource_name: payload.source_id, ...payload }),
  cancel: (id) => API.cancelJob(id),
  retry: (id) => API.retryJob(id),
  logs: (id) => API.jobLogs(id)
};
