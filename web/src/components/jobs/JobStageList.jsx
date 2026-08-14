import { Icon } from '../ui.jsx';
import { titleCase } from '../../format.js';

export function JobStageList({ job = {} }) {
  const stages = stageNames(job.job_type || 'backup');
  const current = Math.max(1, Number(job.stage_index || 1));

  return (
    <ol className="job-stage-list">
      {stages.map((stage, index) => {
        const pos = index + 1;
        const state = job.status === 'failed' && pos === current ? 'failed' : pos < current ? 'done' : pos === current ? 'current' : 'waiting';
        const iconName = state === 'done' ? 'check' : state === 'failed' ? 'warning' : 'clock';

        return (
          <li key={stage} className={state}>
            <span className="stage-marker">
              <Icon name={iconName} size={14} />
            </span>
            <div>
              <strong>{titleCase(stage.replaceAll('_', ' '))}</strong>
              <small>{state === 'done' ? 'Completed' : state === 'current' ? 'In progress' : state === 'failed' ? 'Needs attention' : 'Waiting'}</small>
            </div>
          </li>
        );
      })}
    </ol>
  );
}

function stageNames(type) {
  return {
    backup: ['snapshot', 'inspect', 'plan', 'upload', 'publish', 'verify', 'replicate', 'complete'],
    restore: ['plan', 'reserve', 'download', 'verify', 'restore', 'replay_logs', 'validate', 'complete'],
    restore_drill: ['create_workspace', 'restore', 'open_database', 'run_checks', 'record_evidence', 'cleanup', 'complete'],
    sandbox: ['reserve', 'create_runtime', 'restore', 'verify', 'publish_connection', 'schedule_expiry', 'complete'],
    replication: ['plan_missing_objects', 'copy', 'verify_replica', 'update_coverage', 'complete'],
    doctor: ['database', 'tools', 'storage', 'keys', 'disk', 'clock', 'tls', 'agent', 'restore_requirements', 'complete'],
    bundle: ['collect', 'redact', 'package', 'sign', 'verify', 'complete']
  }[type] || ['queued', 'running', 'complete'];
}
