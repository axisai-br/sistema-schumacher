'use strict';

module.exports = async function authorize({github, context, core}) {
  core.setOutput('deploy-allowed', 'false');
  if (context.eventName !== 'push' || context.ref !== 'refs/heads/main') return;
  const reject = () => { throw new Error('Production source refused: invalid or replayed merge transition.'); };
  const fullSHA = value => typeof value === 'string' && /^[0-9a-f]{40}$/.test(value);
  const {before, after, forced, created, deleted} = context.payload;
  const sha = context.sha;
  if (!fullSHA(sha) || !fullSHA(after) || after !== sha ||
      !fullSHA(before) || /^0+$/.test(before) ||
      forced !== false || created !== false || deleted !== false) reject();

  const {owner, repo} = context.repo;
  const {data: commit} = await github.rest.repos.getCommit({owner, repo, ref: sha});
  // This repository uses two-parent merge commits. Squash/rebase fail closed.
  if (!fullSHA(commit.sha) || commit.sha !== sha || !Array.isArray(commit.parents) ||
      commit.parents.length !== 2 || !fullSHA(commit.parents[0]?.sha) ||
      !fullSHA(commit.parents[1]?.sha) || commit.parents[0].sha !== before) reject();
  const pulls = await github.paginate(github.rest.repos.listPullRequestsAssociatedWithCommit,
    {owner, repo, commit_sha: sha, per_page: 100});
  const candidates = pulls.filter(p => p.merge_commit_sha === sha &&
    p.merged_at && p.base?.ref === 'main' && p.base?.repo?.full_name === owner + '/' + repo);
  if (candidates.length !== 1) reject();
  const {data: pull} = await github.rest.pulls.get({owner, repo, pull_number: candidates[0].number});
  if (pull.merged !== true || !pull.merged_at || !fullSHA(pull.merge_commit_sha) ||
      pull.merge_commit_sha !== sha || !fullSHA(pull.head?.sha) ||
      pull.base?.ref !== 'main' || pull.base?.repo?.full_name !== owner + '/' + repo ||
      pull.head?.sha !== commit.parents[1].sha) reject();

  // Parentage alone also accepts reset-to-parent followed by a historical replay.
  // Only the original push run is eligible, never a new run for the same merge
  // or a rerun. created_at is immutable; queue/build delay does not affect it.
  const {data: run} = await github.rest.actions.getWorkflowRun({owner, repo, run_id: context.runId});
  const age = Date.parse(run.created_at) - Date.parse(pull.merged_at);
  if (run.id !== context.runId || !fullSHA(run.head_sha) || run.head_sha !== sha || run.event !== 'push' ||
      run.head_branch !== 'main' || run.run_attempt !== 1 ||
      !Number.isFinite(age) || age < 0 || age > 120000) reject();
  const {data: history} = await github.rest.actions.listWorkflowRuns({
    owner, repo, workflow_id: run.workflow_id, head_sha: sha,
    branch: 'main', event: 'push', per_page: 100,
  });
  if (history.total_count !== 1 || history.workflow_runs?.length !== 1 ||
      history.workflow_runs[0].id !== run.id) reject();
  core.setOutput('deploy-allowed', 'true');
  core.notice('Production merge transition authorized.');
};
