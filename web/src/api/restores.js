import { API } from '../api.js';

export const RestoresAPI = {
  createDemoRestoreDrill: (sourceId = 'production-postgres') => API.createJob({ job_type: 'restore_drill', resource_id: sourceId, resource_name: 'Production PostgreSQL' })
};
