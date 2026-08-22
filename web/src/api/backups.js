import { API } from '../api.js';

export const BackupsAPI = {
  createDemoBackup: (sourceId = 'production-postgres') => API.createJob({ job_type: 'backup', resource_id: sourceId, resource_name: 'Production PostgreSQL' })
};
