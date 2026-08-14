export const JobsAPI = {
  list: (params = {}) => API.jobs(params),
  detail: (id) => API.job(id),
  create: (payload = {}) => API.createJob(payload),
  cancel: (id) => API.cancelJob(id),
  retry: (id) => API.retryJob(id),
  logs: (id) => API.jobLogs(id)
};
